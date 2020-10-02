// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.

package mongo

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mendersoftware/go-lib-micro/log"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/pkg/errors"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
)

const (
	DbVersion = "1.0.0"

	DbName        = "inventory"
	DbDevicesColl = "devices"

	DbDevId              = "_id"
	DbDevAttributes      = "attributes"
	DbDevGroup           = "group"
	DbDevAttributesDesc  = "description"
	DbDevAttributesValue = "value"
	DbDevAttributesScope = "scope"
	DbDevAttributesName  = "name"
	DbDevAttributesGroup = DbDevAttributes + "." +
		model.AttrScopeSystem + "-" + model.AttrNameGroup
	DbDevAttributesGroupValue = DbDevAttributesGroup + "." +
		DbDevAttributesValue

	DbScopeInventory = "inventory"
)

var (
	//with offcial mongodb supported driver we keep client
	clientGlobal *mongo.Client

	// once ensures client is created only once
	once sync.Once

	ErrNotFound = errors.New("mongo: no documents in result")
)

type DataStoreMongoConfig struct {
	// connection string
	ConnectionString string

	// SSL support
	SSL           bool
	SSLSkipVerify bool

	// Overwrites credentials provided in connection string if provided
	Username string
	Password string
}

type DataStoreMongo struct {
	client      *mongo.Client
	automigrate bool
}

func NewDataStoreMongoWithSession(client *mongo.Client) store.DataStore {
	return &DataStoreMongo{client: client}
}

//config.ConnectionString must contain a valid
func NewDataStoreMongo(config DataStoreMongoConfig) (store.DataStore, error) {
	//init master session
	var err error
	once.Do(func() {
		if !strings.Contains(config.ConnectionString, "://") {
			config.ConnectionString = "mongodb://" + config.ConnectionString
		}
		clientOptions := mopts.Client().ApplyURI(config.ConnectionString)

		if config.Username != "" {
			clientOptions.SetAuth(mopts.Credential{
				Username: config.Username,
				Password: config.Password,
			})
		}

		if config.SSL {
			tlsConfig := &tls.Config{}
			tlsConfig.InsecureSkipVerify = config.SSLSkipVerify
			clientOptions.SetTLSConfig(tlsConfig)
		}

		ctx := context.Background()
		l := log.FromContext(ctx)
		clientGlobal, err = mongo.Connect(ctx, clientOptions)
		if err != nil {
			l.Errorf("mongo: error connecting to mongo '%s'", err.Error())
			return
		}
		if clientGlobal == nil {
			l.Errorf("mongo: client is nil. wow.")
			return
		}
		// from: https://www.mongodb.com/blog/post/mongodb-go-driver-tutorial
		/*
			It is best practice to keep a client that is connected to MongoDB around so that the application can make use of connection pooling - you don't want to open and close a connection for each query. However, if your application no longer requires a connection, the connection can be closed with client.Disconnect() like so:
		*/
		err = clientGlobal.Ping(ctx, nil)
		if err != nil {
			clientGlobal = nil
			l.Errorf("mongo: error pinging mongo '%s'", err.Error())
			return
		}
		if clientGlobal == nil {
			l.Errorf("mongo: global instance of client is nil.")
			return
		}
	})

	if clientGlobal == nil {
		return nil, errors.New("failed to open mongo-driver session")
	}
	db := &DataStoreMongo{client: clientGlobal}

	return db, nil
}

func (db *DataStoreMongo) Ping(ctx context.Context) error {
	res := db.client.Database(DbName).RunCommand(ctx, bson.M{"ping": 1})
	return res.Err()
}

func (db *DataStoreMongo) GetDevices(ctx context.Context, q store.ListQuery) ([]model.Device, int, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	queryFilters := make([]bson.M, 0)
	for _, filter := range q.Filters {
		op := mongoOperator(filter.Operator)
		name := fmt.Sprintf("%s-%s", filter.AttrScope, model.GetDeviceAttributeNameReplacer().Replace(filter.AttrName))
		field := fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
		switch filter.Operator {
		default:
			if filter.ValueFloat != nil {
				queryFilters = append(queryFilters, bson.M{"$or": []bson.M{
					{field: bson.M{op: filter.Value}},
					{field: bson.M{op: filter.ValueFloat}},
				}})
			} else {
				queryFilters = append(queryFilters, bson.M{field: bson.M{op: filter.Value}})
			}
		}
	}
	if q.GroupName != "" {
		groupFilter := bson.M{DbDevAttributesGroupValue: q.GroupName}
		queryFilters = append(queryFilters, groupFilter)
	}
	if q.GroupName != "" {
		groupFilter := bson.M{DbDevAttributesGroupValue: q.GroupName}
		queryFilters = append(queryFilters, groupFilter)
	}
	if q.HasGroup != nil {
		groupExistenceFilter := bson.M{
			DbDevAttributesGroup: bson.M{
				"$exists": *q.HasGroup,
			},
		}
		queryFilters = append(queryFilters, groupExistenceFilter)
	}

	findQuery := bson.M{}
	if len(queryFilters) > 0 {
		findQuery["$and"] = queryFilters
	}

	findOptions := mopts.Find()
	if q.Skip > 0 {
		findOptions.SetSkip(int64(q.Skip))
	}
	if q.Limit > 0 {
		findOptions.SetLimit(int64(q.Limit))
	}
	if q.Sort != nil {
		name := fmt.Sprintf("%s-%s", q.Sort.AttrScope, model.GetDeviceAttributeNameReplacer().Replace(q.Sort.AttrName))
		sortField := fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
		sortFieldQuery := bson.M{}
		sortFieldQuery[sortField] = 1
		if !q.Sort.Ascending {
			sortFieldQuery[sortField] = -1
		}
		findOptions.SetSort(sortFieldQuery)
	}

	cursor, err := c.Find(ctx, findQuery, findOptions)
	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to search devices")
	}
	defer cursor.Close(ctx)

	devices := []model.Device{}
	if err = cursor.All(ctx, &devices); err != nil {
		return nil, -1, errors.Wrap(err, "failed to search devices")
	}

	count, err := c.CountDocuments(ctx, findQuery)
	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to count devices")
	}

	return devices, int(count), nil
}

func (db *DataStoreMongo) GetDevice(
	ctx context.Context,
	id model.DeviceID,
) (*model.Device, error) {
	var res model.Device
	c := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(DbDevicesColl)
	l := log.FromContext(ctx)

	if id == model.NilDeviceID {
		return nil, nil
	}
	if err := c.FindOne(ctx, bson.M{DbDevId: id}).Decode(&res); err != nil {
		switch err {
		case mongo.ErrNoDocuments:
			return nil, nil
		default:
			l.Errorf("GetDevice: %v", err)
			return nil, errors.Wrap(err, "failed to fetch device")
		}
	}
	return &res, nil
}

// AddDevice inserts a new device, initializing the inventory data.
func (db *DataStoreMongo) AddDevice(ctx context.Context, dev *model.Device) error {
	if dev.Group != "" {
		dev.Attributes = append(dev.Attributes, model.DeviceAttribute{
			Scope: model.AttrScopeSystem,
			Name:  model.AttrNameGroup,
			Value: dev.Group,
		})
	}
	_, err := db.UpsertDevicesAttributes(
		ctx, []model.DeviceID{dev.ID}, dev.Attributes,
	)
	if err != nil {
		return errors.Wrap(err, "failed to store device")
	}
	return nil
}

func (db *DataStoreMongo) UpsertDevicesAttributes(
	ctx context.Context,
	ids []model.DeviceID,
	attrs model.DeviceAttributes,
) (*model.UpdateResult, error) {
	const systemScope = DbDevAttributes + "." + model.AttrScopeSystem
	const updatedField = systemScope + "-" + model.AttrNameUpdated
	const createdField = systemScope + "-" + model.AttrNameCreated
	var (
		result *model.UpdateResult
		err    error
	)

	c := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(DbDevicesColl)

	update, err := makeAttrUpsert(attrs)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	update[updatedField] = model.DeviceAttribute{
		Scope: model.AttrScopeSystem,
		Name:  model.AttrNameUpdated,
		Value: now,
	}
	update = bson.M{
		"$set": update,
		"$setOnInsert": bson.M{
			createdField: model.DeviceAttribute{
				Scope: model.AttrScopeSystem,
				Name:  model.AttrNameCreated,
				Value: now,
			},
		},
	}

	switch len(ids) {
	case 0:
		return &model.UpdateResult{}, nil
	case 1:
		var res *mongo.UpdateResult
		filter := map[string]interface{}{"_id": ids[0]}
		res, err = c.UpdateOne(ctx, filter, update, mopts.Update().SetUpsert(true))
		result = &model.UpdateResult{
			MatchedCount: res.MatchedCount,
			CreatedCount: res.UpsertedCount,
		}
	default:
		var bres *mongo.BulkWriteResult
		// Perform single bulk-write operation
		// NOTE: Can't use UpdateMany as $in query operator does not
		//       upsert missing devices.
		models := make([]mongo.WriteModel, len(ids))
		for i, id := range ids {
			umod := mongo.NewUpdateOneModel()
			umod.Filter = bson.M{"_id": id}
			umod.SetUpsert(true)
			umod.Update = update
			models[i] = umod
		}
		bres, err = c.BulkWrite(
			ctx, models, mopts.BulkWrite().SetOrdered(false),
		)
		result = &model.UpdateResult{
			MatchedCount: bres.MatchedCount,
			CreatedCount: bres.UpsertedCount,
		}
	}
	return result, err
}

// makeAttrField is a convenience function for composing attribute field names.
func makeAttrField(attrName, attrScope string, subFields ...string) string {
	field := fmt.Sprintf("%s.%s-%s", DbDevAttributes, attrScope, model.GetDeviceAttributeNameReplacer().Replace(attrName))
	if len(subFields) > 0 {
		field = strings.Join(
			append([]string{field}, subFields...), ".",
		)
	}
	return field
}

// makeAttrUpsert creates a new upsert document for the given attributes.
func makeAttrUpsert(attrs model.DeviceAttributes) (bson.M, error) {
	var fieldName string
	upsert := make(bson.M)

	for i := range attrs {
		if attrs[i].Name == "" {
			return nil, store.ErrNoAttrName
		}
		if attrs[i].Scope == "" {
			// Default to inventory scope
			attrs[i].Scope = model.AttrScopeInventory
		}

		fieldName = makeAttrField(
			attrs[i].Name,
			attrs[i].Scope,
			DbDevAttributesScope,
		)
		upsert[fieldName] = attrs[i].Scope

		fieldName = makeAttrField(
			attrs[i].Name,
			attrs[i].Scope,
			DbDevAttributesName,
		)
		upsert[fieldName] = attrs[i].Name

		if attrs[i].Value != nil {
			fieldName = makeAttrField(
				attrs[i].Name,
				attrs[i].Scope,
				DbDevAttributesValue,
			)
			upsert[fieldName] = attrs[i].Value
		}

		if attrs[i].Description != nil {
			fieldName = makeAttrField(
				attrs[i].Name,
				attrs[i].Scope,
				DbDevAttributesDesc,
			)
			upsert[fieldName] = attrs[i].Description

		}
	}
	return upsert, nil
}

// makeAttrUpsert creates a new upsert document for the given attributes.
func makeAttrRemove(attrs model.DeviceAttributes) (bson.M, error) {
	var fieldName string
	remove := make(bson.M)

	if attrs != nil {
		for i := range attrs {
			if attrs[i].Name == "" {
				return nil, store.ErrNoAttrName
			}
			if attrs[i].Scope == "" {
				// Default to inventory scope
				attrs[i].Scope = model.AttrScopeInventory
			}
			fieldName = makeAttrField(
				attrs[i].Name,
				attrs[i].Scope,
			)
			remove[fieldName] = true
		}
	}
	return remove, nil
}

func mongoOperator(co store.ComparisonOperator) string {
	switch co {
	case store.Eq:
		return "$eq"
	}
	return ""
}

func (db *DataStoreMongo) UpsertRemoveDeviceAttributes(
	ctx context.Context,
	id model.DeviceID,
	updateAttrs model.DeviceAttributes,
	removeAttrs model.DeviceAttributes,
) (*model.UpdateResult, error) {
	const systemScope = DbDevAttributes + "." + model.AttrScopeSystem
	const updatedField = systemScope + "-" + model.AttrNameUpdated
	const createdField = systemScope + "-" + model.AttrNameCreated
	var (
		result *model.UpdateResult
		err    error
	)

	c := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(DbDevicesColl)

	update, err := makeAttrUpsert(updateAttrs)
	if err != nil {
		return nil, err
	}
	remove, err := makeAttrRemove(removeAttrs)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	update[updatedField] = model.DeviceAttribute{
		Scope: model.AttrScopeSystem,
		Name:  model.AttrNameUpdated,
		Value: now,
	}
	update = bson.M{
		"$set": update,
		"$setOnInsert": bson.M{
			createdField: model.DeviceAttribute{
				Scope: model.AttrScopeSystem,
				Name:  model.AttrNameCreated,
				Value: now,
			},
		},
	}
	if len(remove) > 0 {
		update["$unset"] = remove
	}

	var res *mongo.UpdateResult
	filter := map[string]interface{}{"_id": id}
	res, err = c.UpdateOne(ctx, filter, update, mopts.Update().SetUpsert(true))
	if err == nil {
		result = &model.UpdateResult{
			MatchedCount: res.MatchedCount,
			CreatedCount: res.UpsertedCount,
		}
	}
	return result, err
}

func (db *DataStoreMongo) UpdateDevicesGroup(
	ctx context.Context,
	devIDs []model.DeviceID,
	group model.GroupName,
) (*model.UpdateResult, error) {
	database := db.client.Database(mstore.DbFromContext(ctx, DbName))
	collDevs := database.Collection(DbDevicesColl)

	var filter = bson.M{}
	switch len(devIDs) {
	case 0:
		return &model.UpdateResult{}, nil
	case 1:
		filter[DbDevId] = devIDs[0]
	default:
		filter[DbDevId] = bson.M{"$in": devIDs}
	}
	update := bson.M{
		"$set": bson.M{
			DbDevAttributesGroup: model.DeviceAttribute{
				Scope: model.AttrScopeSystem,
				Name:  DbDevGroup,
				Value: group,
			},
		},
	}
	res, err := collDevs.UpdateMany(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	return &model.UpdateResult{
		MatchedCount: res.MatchedCount,
		UpdatedCount: res.ModifiedCount,
	}, nil
}

func (db *DataStoreMongo) UnsetDevicesGroup(
	ctx context.Context,
	deviceIDs []model.DeviceID,
	group model.GroupName,
) (*model.UpdateResult, error) {
	database := db.client.Database(mstore.DbFromContext(ctx, DbName))
	collDevs := database.Collection(DbDevicesColl)

	var filter bson.D
	// Add filter on device id (either $in or direct indexing)
	switch len(deviceIDs) {
	case 0:
		return &model.UpdateResult{}, nil
	case 1:
		filter = bson.D{{Key: DbDevId, Value: deviceIDs[0]}}
	default:
		filter = bson.D{{Key: DbDevId, Value: bson.M{"$in": deviceIDs}}}
	}
	// Append filter on group
	filter = append(
		filter,
		bson.E{Key: DbDevAttributesGroupValue, Value: group},
	)
	// Create unset operation on group attribute
	update := bson.M{
		"$unset": bson.M{
			DbDevAttributesGroup: "",
		},
	}
	res, err := collDevs.UpdateMany(ctx, filter, update)
	if err != nil {
		return nil, err
	}
	return &model.UpdateResult{
		MatchedCount: res.MatchedCount,
		UpdatedCount: res.ModifiedCount,
	}, nil
}

func predicateToQuery(pred model.FilterPredicate) (bson.D, error) {
	if err := pred.Validate(); err != nil {
		return nil, err
	}
	name := fmt.Sprintf(
		"%s.%s-%s.value", DbDevAttributes, pred.Scope, model.GetDeviceAttributeNameReplacer().Replace(pred.Attribute),
	)
	return bson.D{{
		Key: name, Value: bson.D{{Key: pred.Type, Value: pred.Value}},
	}}, nil
}

func (db *DataStoreMongo) ListGroups(
	ctx context.Context,
	filters []model.FilterPredicate,
) ([]model.GroupName, error) {
	c := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(DbDevicesColl)

	fltr := bson.D{{
		Key: DbDevAttributesGroupValue, Value: bson.M{"$exists": true},
	}}
	if len(fltr) > 0 {
		for _, p := range filters {
			q, err := predicateToQuery(p)
			if err != nil {
				return nil, errors.Wrap(
					err, "store: bad filter predicate",
				)
			}
			fltr = append(fltr, q...)
		}
	}
	results, err := c.Distinct(
		ctx, DbDevAttributesGroupValue, fltr,
	)
	if err != nil {
		return nil, err
	}

	groups := make([]model.GroupName, len(results))
	for i, d := range results {
		groups[i] = model.GroupName(d.(string))
	}
	return groups, nil
}

func (db *DataStoreMongo) GetDevicesByGroup(ctx context.Context, group model.GroupName, skip, limit int) ([]model.DeviceID, int, error) {
	c := db.client.
		Database(mstore.DbFromContext(ctx, DbName)).
		Collection(DbDevicesColl)

	filter := bson.M{DbDevAttributesGroupValue: group}
	result := c.FindOne(ctx, filter)
	if result == nil {
		return nil, -1, store.ErrGroupNotFound
	}

	var dev model.Device
	err := result.Decode(&dev)
	if err != nil {
		return nil, -1, store.ErrGroupNotFound
	}

	hasGroup := group != ""
	devices, totalDevices, e := db.GetDevices(ctx,
		store.ListQuery{
			Skip:      skip,
			Limit:     limit,
			Filters:   nil,
			Sort:      nil,
			HasGroup:  &hasGroup,
			GroupName: string(group)})
	if e != nil {
		return nil, -1, errors.Wrap(e, "failed to get device list for group")
	}

	resIds := make([]model.DeviceID, len(devices))
	for i, d := range devices {
		resIds[i] = d.ID
	}
	return resIds, totalDevices, nil
}

func (db *DataStoreMongo) GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error) {
	dev, err := db.GetDevice(ctx, id)
	if err != nil || dev == nil {
		return "", store.ErrDevNotFound
	}

	return dev.Group, nil
}

func (db *DataStoreMongo) DeleteDevices(
	ctx context.Context, ids []model.DeviceID,
) (*model.UpdateResult, error) {
	var filter = bson.M{}
	database := db.client.Database(mstore.DbFromContext(ctx, DbName))
	collDevs := database.Collection(DbDevicesColl)

	switch len(ids) {
	case 0:
		// This is a no-op, don't bother requesting mongo.
		return &model.UpdateResult{DeletedCount: 0}, nil
	case 1:
		filter[DbDevId] = ids[0]
	default:
		filter[DbDevId] = bson.M{"$in": ids}
	}
	res, err := collDevs.DeleteMany(ctx, filter)
	if err != nil {
		return nil, err
	}
	return &model.UpdateResult{
		DeletedCount: res.DeletedCount,
	}, nil
}

func (db *DataStoreMongo) GetAllAttributeNames(ctx context.Context) ([]string, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	project := bson.M{
		"$project": bson.M{
			"arrayofkeyvalue": bson.M{
				"$objectToArray": "$$ROOT.attributes",
			},
		},
	}

	unwind := bson.M{
		"$unwind": "$arrayofkeyvalue",
	}

	group := bson.M{
		"$group": bson.M{
			"_id": nil,
			"allkeys": bson.M{
				"$addToSet": "$arrayofkeyvalue.v.name",
			},
		},
	}

	l := log.FromContext(ctx)
	cursor, err := c.Aggregate(ctx, []bson.M{
		project,
		unwind,
		group,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	cursor.Next(ctx)
	elem := &bson.D{}
	err = cursor.Decode(elem)
	if err != nil {
		if err != io.EOF {
			return nil, errors.Wrap(err, "failed to get attributes")
		} else {
			return make([]string, 0), nil
		}
	}
	m := elem.Map()
	results := m["allkeys"].(primitive.A)
	attributeNames := make([]string, len(results))
	for i, d := range results {
		attributeNames[i] = d.(string)
		l.Debugf("GetAllAttributeNames got: '%v'", d)
	}

	return attributeNames, nil
}

func (db *DataStoreMongo) SearchDevices(ctx context.Context, searchParams model.SearchParams) ([]model.Device, int, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	queryFilters := make([]bson.M, 0)
	for _, filter := range searchParams.Filters {
		op := filter.Type
		var field string
		if filter.Scope == model.AttrScopeIdentity && filter.Attribute == model.AttrNameID {
			field = DbDevId
		} else {
			name := fmt.Sprintf("%s-%s", filter.Scope, model.GetDeviceAttributeNameReplacer().Replace(filter.Attribute))
			field = fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
		}
		queryFilters = append(queryFilters, bson.M{field: bson.M{op: filter.Value}})
	}

	// FIXME: remove after migrating ids to attributes
	if len(searchParams.DeviceIDs) > 0 {
		queryFilters = append(queryFilters, bson.M{"_id": bson.M{"$in": searchParams.DeviceIDs}})
	}

	findQuery := bson.M{}
	if len(queryFilters) > 0 {
		findQuery["$and"] = queryFilters
	}

	findOptions := mopts.Find()
	findOptions.SetSkip(int64((searchParams.Page - 1) * searchParams.PerPage))
	findOptions.SetLimit(int64(searchParams.PerPage))

	if len(searchParams.Sort) > 0 {
		sortField := bson.M{}
		for _, sortQ := range searchParams.Sort {
			name := fmt.Sprintf("%s-%s", sortQ.Scope, model.GetDeviceAttributeNameReplacer().Replace(sortQ.Attribute))
			field := fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
			sortField[field] = 1
			if sortQ.Order == "desc" {
				sortField[field] = -1
			}
		}
		findOptions.SetSort(sortField)
	}

	cursor, err := c.Find(ctx, findQuery, findOptions)
	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to search devices")
	}
	defer cursor.Close(ctx)

	devices := []model.Device{}

	if err = cursor.All(ctx, &devices); err != nil {
		return nil, -1, errors.Wrap(err, "failed to search devices")
	}

	count, err := c.CountDocuments(ctx, findQuery)

	return devices, int(count), nil
}

func indexAttr(s *mongo.Client, ctx context.Context, attr string) error {
	l := log.FromContext(ctx)
	c := s.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
	indexField := fmt.Sprintf("attributes.%s.values", attr)

	indexView := c.Indexes()
	_, err := indexView.CreateOne(ctx, mongo.IndexModel{Keys: bson.M{indexField: 1}, Options: nil})

	if err != nil {
		if isTooManyIndexes(err) {
			l.Warnf("failed to index attr %s in db %s: too many indexes", attr, mstore.DbFromContext(ctx, DbName))
		} else {
			return errors.Wrapf(err, "failed to index attr %s in db %s", attr, mstore.DbFromContext(ctx, DbName))
		}
	}

	return nil
}

func isTooManyIndexes(e error) bool {
	return strings.HasPrefix(e.Error(), "add index fails, too many indexes for inventory.devices")
}

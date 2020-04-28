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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
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
		clientOptions := options.Client().ApplyURI(config.ConnectionString)

		if config.Username != "" {
			clientOptions.SetAuth(options.Credential{
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

func (db *DataStoreMongo) GetDevices(ctx context.Context, q store.ListQuery) ([]model.Device, int, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	if q.GroupName != "" {
		q.Filters = append(q.Filters, store.Filter{
			AttrName:   "group",
			AttrScope:  "identity",
			Value:      q.GroupName,
			ValueFloat: nil,
			Operator:   store.Eq,
		})
	}
	queryFilters := make([]bson.M, 0)
	for _, filter := range q.Filters {
		op := mongoOperator(filter.Operator)
		name := fmt.Sprintf("%s-%s", filter.AttrScope, filter.AttrName)
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
	findQuery := bson.M{}
	if len(queryFilters) > 0 {
		findQuery["$and"] = queryFilters
	}
	groupExistenceFilter := bson.M{}
	if q.HasGroup != nil {
		groupExistenceFilter = bson.M{"attributes.identity-group.value": bson.M{"$exists": *q.HasGroup}}
	}
	filter := bson.M{
		"$match": bson.M{
			"$and": []bson.M{
				groupExistenceFilter,
				findQuery,
			},
		},
	}

	// since the sorting step will have to be executable we have to use a noop here instead of just
	// an empty query object, as unsorted queries would fail otherwise
	sortQuery := bson.M{"$skip": 0}
	if q.Sort != nil {
		name := fmt.Sprintf("%s-%s", q.Sort.AttrScope, q.Sort.AttrName)
		sortField := fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
		sortFieldQuery := bson.M{}
		sortFieldQuery[sortField] = 1
		if !q.Sort.Ascending {
			sortFieldQuery[sortField] = -1
		}
		sortQuery = bson.M{"$sort": sortFieldQuery}
	}
	limitQuery := bson.M{"$skip": 0}
	// exchange the limit query only if limit is set, as limits need to be positive in an aggregation pipeline
	if q.Limit > 0 {
		limitQuery = bson.M{"$limit": q.Limit}
	}
	combinedQuery := bson.M{
		"$facet": bson.M{
			"results": []bson.M{
				sortQuery,
				bson.M{"$skip": q.Skip},
				limitQuery,
			},
			"totalCount": []bson.M{
				bson.M{"$count": "count"},
			},
		},
	}
	resultMap := bson.M{
		"$project": bson.M{
			"results": 1,
			"totalCount": bson.M{
				"$ifNull": []interface{}{
					bson.M{
						"$arrayElemAt": []interface{}{"$totalCount.count", 0},
					},
					0,
				},
			},
		},
	}

	cursor, err := c.Aggregate(ctx, []bson.M{
		filter,
		combinedQuery,
		resultMap,
	})
	defer cursor.Close(ctx)

	cursor.Next(ctx)
	elem := &bson.D{}
	if err = cursor.Decode(elem); err != nil {
		return nil, -1, errors.Wrap(err, "failed to fetch device list")
	}
	m := elem.Map()
	count := m["totalCount"].(int32)
	results := m["results"].(primitive.A)
	devices := make([]model.Device, len(results))
	for i, d := range results {
		var device model.Device
		bsonBytes, e := bson.Marshal(d.(bson.D))
		if e != nil {
			return nil, int(count), errors.Wrap(e, "failed to parse device in device list")
		}
		bson.Unmarshal(bsonBytes, &device)
		devices[i] = device
	}

	return devices, int(count), nil
}

func (db *DataStoreMongo) GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	l := log.FromContext(ctx)
	var res model.Device

	if id == model.NilDeviceID {
		return nil, nil
	}
	cursor, err := c.Find(ctx, bson.M{DbDevId: id})
	if err != nil {
		l.Errorf("GetDevice returns '%s'", err.Error())
		return nil, err
	}

	cursor.Next(ctx)
	err = cursor.Decode(&res)
	if err != nil {
		if err == io.EOF {
			l.Errorf("GetDevice returns nil,nil")
			return nil, nil
		} else {
			l.Errorf("GetDevice returns nil,'%v' 'failed to fetch device'", err)
			return nil, errors.Wrap(err, "failed to fetch device")
		}
	}
	return &res, nil
}

func (db *DataStoreMongo) AddDevice(ctx context.Context, dev *model.Device) error {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
	filter := bson.M{"_id": dev.ID}
	update := makeAttrUpsert(dev.Attributes)
	now := time.Now()
	update["updated_ts"] = now
	update = bson.M{"$set": update,
		"$setOnInsert": bson.M{"created_ts": now}}
	l := log.FromContext(ctx)
	l.Debugf("upserting: '%s'->'%s'.", filter, update)
	res, err := c.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true)) //this does not insert anything else than ID from model.Device
	if err != nil {
		return errors.Wrap(err, "failed to store device")
	}
	if res.ModifiedCount < 1 {
		return errors.Wrap(err, "failed to store device")
	} // to check the update count
	return nil
}

func (db *DataStoreMongo) SetAttribute(ctx context.Context, id model.DeviceID, attr model.DeviceAttribute) error {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
	filter := bson.M{"_id": id}
	update := makeAttrSet(attr)
	update["updated_ts"] = time.Now()
	update = bson.M{"$set": update}
	res, err := c.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount < 1 {
		return store.ErrDevNotFound
	}
	return nil
}

func (db *DataStoreMongo) UnSetAttribute(ctx context.Context, id model.DeviceID, attr model.DeviceAttribute) error {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
	filter := makeAttrSet(attr)
	filter["_id"] = id
	update := makeAttrSet(attr)
	//update["updated_ts"] = time.Now()
	update = bson.M{"$unset": update}
	res, err := c.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount < 1 {
		return store.ErrDevOrAttrNotFound
	}
	return nil
}

func (db *DataStoreMongo) UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
	filter := bson.M{"_id": id} // idDev}
	update := makeAttrUpsert(attrs)
	now := time.Now()
	update["updated_ts"] = now
	update = bson.M{"$set": update,
		"$setOnInsert": bson.M{"created_ts": now}}
	_, err := c.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		return err
	}
	return nil
}

// prepare an attribute upsert doc based on DeviceAttribute
func makeAttrSet(a model.DeviceAttribute) map[string]interface{} {
	var fieldName string
	upsert := map[string]interface{}{}

	name := a.Name
	if a.Scope != "" {
		// prefix attribute name with a scope
		name = fmt.Sprintf("%s-%s", a.Scope, name)
		fieldName =
			fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesScope)
		upsert[fieldName] = a.Scope
	}

	if a.Description != nil {
		fieldName =
			fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesDesc)
		upsert[fieldName] = a.Description

	}

	if a.Value != nil {
		fieldName =
			fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
		upsert[fieldName] = a.Value
	}

	if a.Name != "" {
		fieldName =
			fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, "name")
		upsert[fieldName] = a.Name
	}

	return upsert
}

// prepare an attribute upsert doc based on DeviceAttributes map
func makeAttrUpsert(attrs model.DeviceAttributes) map[string]interface{} {
	var fieldName string
	upsert := map[string]interface{}{}

	for name, a := range attrs {
		if a.Scope != "" {
			// prefix attribute name with a scope
			name = fmt.Sprintf("%s-%s", a.Scope, name)
			fieldName =
				fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesScope)
			upsert[fieldName] = a.Scope
		}

		if a.Description != nil {
			fieldName =
				fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesDesc)
			upsert[fieldName] = a.Description

		}

		if a.Value != nil {
			fieldName =
				fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, DbDevAttributesValue)
			upsert[fieldName] = a.Value
		}

		if a.Name != "" {
			fieldName =
				fmt.Sprintf("%s.%s.%s", DbDevAttributes, name, "name")
			upsert[fieldName] = a.Name
		}
	}

	return upsert
}

func mongoOperator(co store.ComparisonOperator) string {
	switch co {
	case store.Eq:
		return "$eq"
	}
	return ""
}

func (db *DataStoreMongo) UnsetDeviceGroup(ctx context.Context, id model.DeviceID, groupName model.GroupName) error {
	err := db.UnSetAttribute(ctx, id, model.DeviceAttribute{
		Name:        "group",
		Description: nil,
		Value:       groupName.String(),
		Scope:       "identity",
	})
	if err != nil {
		return err
	}
	return nil
}

func (db *DataStoreMongo) UpdateDeviceGroup(ctx context.Context, devId model.DeviceID, newGroup model.GroupName) error {
	return db.SetAttribute(ctx, devId, model.DeviceAttribute{
		Name:        "group",
		Description: nil,
		Value:       newGroup.String(),
		Scope:       "identity",
	})
}

func (db *DataStoreMongo) ListGroups(ctx context.Context) ([]model.GroupName, error) {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	filter := bson.M{"attributes.identity-group.value": bson.M{"$exists": true}}
	results, err := c.Distinct(ctx, "attributes.identity-group.value", filter)
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
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	filter := bson.M{"attributes.identity-group.value": group}
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
	if err != nil || dev == nil {
		return "", errors.Wrap(err, "failed to get device")
	}

	if _, ok := dev.Attributes["identity-group"]; ok {
		dev.Group = model.GroupName(dev.Attributes["identity-group"].Value.(string))
	}
	return dev.Group, nil
}

func (db *DataStoreMongo) DeleteDevice(ctx context.Context, id model.DeviceID) error {
	c := db.client.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)

	filter := bson.M{DbDevId: id}
	result, err := c.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if result.DeletedCount < 1 {
		return store.ErrDevNotFound
	} // to check the delete count

	return nil
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
				"$addToSet": "$arrayofkeyvalue.k",
			},
		},
	}

	l := log.FromContext(ctx)
	cursor, err := c.Aggregate(ctx, []bson.M{
		project,
		unwind,
		group,
	})
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

func (db *DataStoreMongo) MigrateTenant(ctx context.Context, version string, tenantId string) error {
	l := log.FromContext(ctx)

	database := mstore.DbNameForTenant(tenantId, DbName)

	l.Infof("migrating %s", database)

	m := migrate.SimpleMigrator{
		Client:      db.client,
		Db:          database,
		Automigrate: db.automigrate,
	}

	ver, err := migrate.NewVersion(version)
	if err != nil {
		return errors.Wrap(err, "failed to parse service version")
	}

	ctx = identity.WithContext(ctx, &identity.Identity{
		Tenant: tenantId,
	})

	migrations := []migrate.Migration{
		&migration_0_2_0{
			ms:  db,
			ctx: ctx,
		},
		&migration_1_0_0{
			ms:  db,
			ctx: ctx,
		},
	}

	err = m.Apply(ctx, *ver, migrations)
	if err != nil {
		return errors.Wrap(err, "failed to apply migrations")
	}

	return nil
}

func (db *DataStoreMongo) Migrate(ctx context.Context, version string) error {
	l := log.FromContext(ctx)

	dbs, err := migrate.GetTenantDbs(ctx, db.client, mstore.IsTenantDb(DbName))
	if err != nil {
		return errors.Wrap(err, "failed go retrieve tenant DBs")
	}

	if len(dbs) == 0 {
		dbs = []string{DbName}
	}

	if db.automigrate {
		l.Infof("automigrate is ON, will apply migrations")
	} else {
		l.Infof("automigrate is OFF, will check db version compatibility")
	}

	for _, d := range dbs {
		l.Infof("migrating %s", d)

		tenantId := mstore.TenantFromDbName(d, DbName)

		if err := db.MigrateTenant(ctx, version, tenantId); err != nil {
			return err
		}
	}
	return nil
}

// WithAutomigrate enables automatic migration and returns a new datastore based
// on current one
func (db *DataStoreMongo) WithAutomigrate() store.DataStore {
	return &DataStoreMongo{
		client:      db.client,
		automigrate: true,
	}
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

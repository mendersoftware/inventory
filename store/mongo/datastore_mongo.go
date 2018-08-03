// Copyright 2018 Northern.tech AS
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
	"net"
	"sync"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/log"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/pkg/errors"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
)

const (
	DbVersion = "0.1.0"

	DbName        = "inventory"
	DbDevicesColl = "devices"

	DbDevAttributes      = "attributes"
	DbDevGroup           = "group"
	DbDevAttributesDesc  = "description"
	DbDevAttributesValue = "value"
)

var (
	// masterSession is a master session to be copied on demand
	// This is the preferred pattern with mgo (for common conn pool management, etc.)
	masterSession *mgo.Session

	// once ensures mgoMaster is created only once
	once sync.Once
)

type DataStoreMongoConfig struct {
	// MGO connection string
	ConnectionString string

	// SSL support
	SSL           bool
	SSLSkipVerify bool

	// Overwrites credentials provided in connection string if provided
	Username string
	Password string
}

type DataStoreMongo struct {
	session     *mgo.Session
	automigrate bool
}

func NewDataStoreMongoWithSession(session *mgo.Session) *DataStoreMongo {
	return &DataStoreMongo{session: session}
}

func NewDataStoreMongo(config DataStoreMongoConfig) (*DataStoreMongo, error) {
	//init master session
	var err error
	once.Do(func() {

		var dialInfo *mgo.DialInfo
		dialInfo, err = mgo.ParseURL(config.ConnectionString)
		if err != nil {
			return
		}

		// Set 10s timeout - same as set by Dial
		dialInfo.Timeout = 10 * time.Second

		if config.Username != "" {
			dialInfo.Username = config.Username
		}
		if config.Password != "" {
			dialInfo.Password = config.Password
		}

		if config.SSL {
			dialInfo.DialServer = func(addr *mgo.ServerAddr) (net.Conn, error) {

				// Setup TLS
				tlsConfig := &tls.Config{}
				tlsConfig.InsecureSkipVerify = config.SSLSkipVerify

				conn, err := tls.Dial("tcp", addr.String(), tlsConfig)
				return conn, err
			}
		}

		masterSession, err = mgo.DialWithInfo(dialInfo)
		if err != nil {
			return
		}

		// Validate connection
		if err = masterSession.Ping(); err != nil {
			return
		}

		// force write ack with immediate journal file fsync
		masterSession.SetSafe(&mgo.Safe{
			W: 1,
			J: true,
		})
	})

	if err != nil {
		return nil, errors.Wrap(err, "failed to open mgo session")
	}

	db := &DataStoreMongo{session: masterSession}

	return db, nil
}

func (db *DataStoreMongo) GetDevices(ctx context.Context, q store.ListQuery) ([]model.Device, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)
	res := []model.Device{}

	queryFilters := make([]bson.M, 0)
	for _, filter := range q.Filters {
		op := mongoOperator(filter.Operator)
		field := fmt.Sprintf("%s.%s.%s", DbDevAttributes, filter.AttrName, DbDevAttributesValue)
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

	if q.HasGroup != nil {
		if *q.HasGroup {
			queryFilters = append(queryFilters, bson.M{DbDevGroup: bson.M{"$exists": true}})
		} else {
			queryFilters = append(queryFilters, bson.M{DbDevGroup: bson.M{"$exists": false}})
		}
	}

	if q.GroupName != "" {
		queryFilters = append(queryFilters, bson.M{DbDevGroup: q.GroupName})
	}

	findQuery := bson.M{}
	if len(queryFilters) > 0 {
		findQuery["$and"] = queryFilters
	}

	query := c.Find(findQuery).Skip(q.Skip).Limit(q.Limit)
	if q.Sort != nil {
		sortField := fmt.Sprintf("%s.%s.%s", DbDevAttributes, q.Sort.AttrName, DbDevAttributesValue)
		if q.Sort.Ascending {
			query.Sort(sortField)
		} else {
			query.Sort("-" + sortField)
		}
	}

	err := query.All(&res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch device list")
	}

	return res, nil
}

func (db *DataStoreMongo) GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	res := model.Device{}

	err := c.FindId(id).One(&res)

	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, nil
		} else {
			return nil, errors.Wrap(err, "failed to fetch device")
		}
	}

	return &res, nil
}

func (db *DataStoreMongo) AddDevice(ctx context.Context, dev *model.Device) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	update := makeAttrUpsert(dev.Attributes)
	now := time.Now()
	update["updated_ts"] = now
	update = bson.M{"$set": update,
		"$setOnInsert": bson.M{"created_ts": now}}

	_, err := c.UpsertId(dev.ID, update)
	if err != nil {
		return errors.Wrap(err, "failed to store device")
	}
	return nil
}

func (db *DataStoreMongo) UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	update := makeAttrUpsert(attrs)

	//set update time and optionally created time
	now := time.Now()
	update["updated_ts"] = now
	update = bson.M{"$set": update,
		"$setOnInsert": bson.M{"created_ts": now}}

	_, err := c.UpsertId(id, update)

	return err
}

// prepare an attribute upsert doc based on DeviceAttributes map
func makeAttrUpsert(attrs model.DeviceAttributes) map[string]interface{} {
	var fieldName string
	upsert := map[string]interface{}{}

	for name, a := range attrs {
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
	s := db.session.Copy()
	defer s.Close()

	query := bson.M{
		"_id":   id,
		"group": groupName,
	}
	update := mgo.Change{
		Update: bson.M{
			"$unset": bson.M{
				"group": 1,
			},
		},
	}
	if _, err := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl).Find(query).Apply(update, nil); err != nil {
		if err.Error() == mgo.ErrNotFound.Error() {
			return store.ErrDevNotFound
		}
		return err
	}
	return nil
}

func (db *DataStoreMongo) UpdateDeviceGroup(ctx context.Context, devId model.DeviceID, newGroup model.GroupName) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	err := c.UpdateId(devId, bson.M{"$set": &model.Device{Group: newGroup}})
	if err != nil {
		if err == mgo.ErrNotFound {
			return store.ErrDevNotFound
		}
		return errors.Wrap(err, "failed to update device group")
	}
	return nil
}

func (db *DataStoreMongo) ListGroups(ctx context.Context) ([]model.GroupName, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	var groups []model.GroupName
	err := c.Find(bson.M{}).Distinct("group", &groups)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list device groups")
	}
	return groups, nil
}

func (db *DataStoreMongo) GetDevicesByGroup(ctx context.Context, group model.GroupName, skip, limit int) ([]model.DeviceID, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	filter := bson.M{DbDevGroup: group}

	//first, find if the group exists at all, i.e. if any dev is assigned
	var dev model.Device
	err := c.Find(filter).One(&dev)
	if err != nil {
		if err == mgo.ErrNotFound {
			return nil, store.ErrGroupNotFound
		} else {
			return nil, errors.Wrap(err, "failed to get devices for group")
		}
	}

	res := []model.Device{}

	//get group's devices; select only the '_id' field
	err = c.Find(filter).Select(bson.M{"_id": 1}).Skip(skip).Limit(limit).Sort("_id").All(&res)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices for group")
	}

	resIds := make([]model.DeviceID, len(res))
	for i, d := range res {
		resIds[i] = d.ID
	}

	return resIds, nil
}

func (db *DataStoreMongo) GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)

	var dev model.Device

	err := c.FindId(id).Select(bson.M{"group": 1}).One(&dev)
	if err != nil {
		if err == mgo.ErrNotFound {
			return "", store.ErrDevNotFound
		} else {
			return "", errors.Wrap(err, "failed to get device")
		}
	}

	return dev.Group, nil
}

func (db *DataStoreMongo) DeleteDevice(ctx context.Context, id model.DeviceID) error {
	s := db.session.Copy()
	defer s.Close()

	if err := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl).RemoveId(id); err != nil {
		if err.Error() == mgo.ErrNotFound.Error() {
			return store.ErrDevNotFound
		}
		return err
	}

	return nil
}

func (db *DataStoreMongo) MigrateTenant(ctx context.Context, version string, tenant string) error {
	ver, err := migrate.NewVersion(version)
	if err != nil {
		return errors.Wrap(err, "failed to parse service version")
	}

	tenantCtx := identity.WithContext(ctx, &identity.Identity{
		Tenant: tenant,
	})

	m := migrate.DummyMigrator{
		Session:     db.session,
		Db:          mstore.DbFromContext(tenantCtx, DbName),
		Automigrate: db.automigrate,
	}

	err = m.Apply(tenantCtx, *ver, nil)
	if err != nil {
		return errors.Wrap(err, "failed to apply migrations")
	}
	return nil
}

func (db *DataStoreMongo) Migrate(ctx context.Context, version string) error {
	l := log.FromContext(ctx)

	dbs, err := migrate.GetTenantDbs(db.session, mstore.IsTenantDb(DbName))
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

		// if not in multi tenant, then tenant will be "" and identity
		// will be the same as default
		tenant := mstore.TenantFromDbName(d, DbName)

		if err := db.MigrateTenant(ctx, version, tenant); err != nil {
			return err
		}
	}

	return nil
}

// WithAutomigrate enables automatic migration and returns a new datastore based
// on current one
func (db *DataStoreMongo) WithAutomigrate() *DataStoreMongo {
	return &DataStoreMongo{
		session:     db.session,
		automigrate: true,
	}
}

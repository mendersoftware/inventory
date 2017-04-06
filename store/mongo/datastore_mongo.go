// Copyright 2016 Mender Software AS
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
	"fmt"
	"sync"
	"time"

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

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

type DataStoreMongo struct {
	session *mgo.Session
}

func NewDataStoreMongoWithSession(session *mgo.Session) *DataStoreMongo {
	return &DataStoreMongo{session: session}
}

func NewDataStoreMongo(host string) (*DataStoreMongo, error) {
	//init master session
	var err error
	once.Do(func() {
		masterSession, err = mgo.Dial(host)
		if err == nil {
			// force write ack with immediate journal file fsync
			masterSession.SetSafe(&mgo.Safe{
				WMode: "1",
				J:     true,
			})
		}
	})

	if err != nil {
		return nil, errors.New("failed to open mgo session")
	}

	db := &DataStoreMongo{session: masterSession}

	return db, nil
}

func (db *DataStoreMongo) GetDevices(ctx context.Context, skip int, limit int, filters []store.Filter, sort *store.Sort, hasGroup *bool) ([]model.Device, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)
	res := []model.Device{}

	queryFilters := make([]bson.M, 0)
	for _, filter := range filters {
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

	if hasGroup != nil {
		if *hasGroup {
			queryFilters = append(queryFilters, bson.M{DbDevGroup: bson.M{"$exists": true}})
		} else {
			queryFilters = append(queryFilters, bson.M{DbDevGroup: bson.M{"$exists": false}})
		}
	}

	findQuery := bson.M{}
	if len(queryFilters) > 0 {
		findQuery["$and"] = queryFilters
	}

	query := c.Find(findQuery).Skip(skip).Limit(limit)
	if sort != nil {
		sortField := fmt.Sprintf("%s.%s.%s", DbDevAttributes, sort.AttrName, DbDevAttributesValue)
		if sort.Ascending {
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
	c := s.DB(DbName).C(DbDevicesColl)

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
	c := s.DB(DbName).C(DbDevicesColl)

	err := c.Insert(dev)
	if err != nil {
		if mgo.IsDup(err) {
			return store.ErrDuplicatedDeviceId
		}
		return errors.Wrap(err, "failed to store device")
	}
	return nil
}

func (db *DataStoreMongo) UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)

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
	if _, err := s.DB(DbName).C(DbDevicesColl).Find(query).Apply(update, nil); err != nil {
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
	c := s.DB(DbName).C(DbDevicesColl)

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
	c := s.DB(DbName).C(DbDevicesColl)

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
	c := s.DB(DbName).C(DbDevicesColl)

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
	c := s.DB(DbName).C(DbDevicesColl)

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

	if err := s.DB(DbName).C(DbDevicesColl).RemoveId(id); err != nil {
		if err.Error() == mgo.ErrNotFound.Error() {
			return store.ErrDevNotFound
		}
		return err
	}

	return nil
}

func (db *DataStoreMongo) Migrate(ctx context.Context, version string, migrations []migrate.Migration) error {
	m := migrate.DummyMigrator{
		Session: db.session,
		Db:      DbName,
	}

	ver, err := migrate.NewVersion(version)
	if err != nil {
		return errors.Wrap(err, "failed to parse service version")
	}

	err = m.Apply(ctx, *ver, migrations)
	if err != nil {
		return errors.Wrap(err, "failed to apply migrations")
	}

	return nil
}

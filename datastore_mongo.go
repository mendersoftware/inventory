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

package main

import (
	"fmt"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"sync"
)

const (
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
	})
	if err != nil {
		return nil, errors.New("failed to open mgo session")
	}

	db := &DataStoreMongo{session: masterSession}

	return db, nil
}

func (db *DataStoreMongo) GetDevices(skip int, limit int, filters []Filter, sort *Sort, hasGroup *bool) ([]Device, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)
	res := []Device{}

	findQuery := make(bson.M, 0)
	for _, filter := range filters {
		op := mongoOperator(filter.Operator)
		field := fmt.Sprintf("%s.%s.%s", DbDevAttributes, filter.AttrName, DbDevAttributesValue)
		switch filter.Operator {
		default:
			if filter.ValueFloat != nil {
				findQuery["$or"] = []bson.M{
					bson.M{field: bson.M{op: filter.Value}},
					bson.M{field: bson.M{op: filter.ValueFloat}},
				}
			} else {
				findQuery[field] = bson.M{op: filter.Value}
			}
		}
	}

	if hasGroup != nil {
		if *hasGroup {
			findQuery[DbDevGroup] = bson.M{"$exists": true}
		} else {
			findQuery[DbDevGroup] = bson.M{"$exists": false}
		}
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

func (db *DataStoreMongo) GetDevice(id DeviceID) (*Device, error) {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)

	res := Device{}

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

func (db *DataStoreMongo) AddDevice(dev *Device) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)

	err := c.Insert(dev)
	if err != nil {
		if mgo.IsDup(err) {
			return ErrDuplicatedDeviceId
		}
		return errors.Wrap(err, "failed to store device")
	}
	return nil
}

func (db *DataStoreMongo) UpsertAttributes(id DeviceID, attrs DeviceAttributes) error {
	s := db.session.Copy()
	defer s.Close()
	c := s.DB(DbName).C(DbDevicesColl)

	update := makeAttrUpsert(attrs)
	update = bson.M{"$set": update}

	_, err := c.UpsertId(id, update)

	return err
}

// prepare an attribute upsert doc based on DeviceAttributes map
func makeAttrUpsert(attrs DeviceAttributes) interface{} {
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

func mongoOperator(co ComparisonOperator) string {
	switch co {
	case Eq:
		return "$eq"
	}
	return ""
}

func (db *DataStoreMongo) UnsetDeviceGroup(id DeviceID, groupName GroupName) error {
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
			if devCount, findIdErr := s.DB(DbName).C(DbDevicesColl).FindId(id).Count(); devCount == 0 {
				return ErrDevNotFound
			} else if findIdErr == nil {
				return ErrDevNotInGivenGroup
			}
		}
		return err
	}
	return nil
}

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
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"sync"
)

const (
	DbName        = "inventory"
	DbDevicesColl = "devices"
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

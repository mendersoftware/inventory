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
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	testDataFolder  = "testdata"
	allDevsInputSet = "get_devices_input.json"
)

// these tests need a real instance of mongodb
// hardcoding the address here - valid both for the Travis env and local dev env
const TestDb = "127.0.0.1:27017"

// db and test management funcs
func getDb() (*DataStoreMongo, error) {
	d, err := NewDataStoreMongo(TestDb)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func setUp(db *DataStoreMongo, dataset string) error {
	devs, err := parseDevs(dataset)
	if err != nil {
		return err
	}

	s := db.session.Copy()
	defer s.Close()

	c := s.DB(DbName).C(DbDevicesColl)

	for _, d := range devs {
		err = c.Insert(d)
		if err != nil {
			return err
		}
	}

	return nil
}

func wipe(db *DataStoreMongo) error {
	s := db.session.Copy()
	defer s.Close()

	c := s.DB(DbName).C(DbDevicesColl)

	_, err := c.RemoveAll(nil)
	if err != nil {
		return err
	}

	return nil
}

func parseDevs(dataset string) ([]DeviceDb, error) {
	f, err := os.Open(filepath.Join(testDataFolder, dataset))
	if err != nil {
		return nil, err
	}

	var devs []DeviceDb

	j := json.NewDecoder(f)
	if err = j.Decode(&devs); err != nil {
		return nil, err
	}

	return devs, nil
}

func TestMongoGetDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMongoGetDevice in short mode.")
	}
	t.Parallel()

	d, err := getDb()
	assert.NoError(t, err, "obtaining DB failed")

	dev, err := d.GetDevice("")
	assert.Nil(t, dev, "expected no device to be found")
	assert.NoError(t, err, "expected no error")
}

func TestMongoAddDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMongoGetDevice in short mode.")
	}
	t.Parallel()

	d, err := getDb()
	assert.NoError(t, err, "obtaining DB failed")

	wipe(d)

	// all dataset of all devices
	devs, err := parseDevs(allDevsInputSet)
	assert.NoError(t, err, "failed to parse expected devs %s", allDevsInputSet)

	// insert all devices to DB
	for _, dev := range devs {
		err := d.AddDevice(&dev)
		assert.NoError(t, err, "expected no error inserting to data store")
	}

	// get devices, one by one
	for _, dev := range devs {
		// we expect to find a device that was present in the
		// input set
		dbdev, err := d.GetDevice(dev.ID)
		assert.NoError(t, err, "expected no error")
		assert.NotNil(t, dbdev, "expected to device of ID %s to be found",
			dev.ID)

		// other fields should be identical
		assert.Equal(t, dev.ID, dbdev.ID)

		// TODO check attributes
		assert.True(t, reflect.DeepEqual(dev.Attributes, dbdev.Attributes), "expected attrs %+v to be equal to %+v", dev.Attributes, dbdev.Attributes)

		// obviously the found device should be identical
		assert.True(t, reflect.DeepEqual(dev, *dbdev), "expected dev %+v to be equal to %+v",
			dbdev, dev)
	}

	// wipe(d)
}

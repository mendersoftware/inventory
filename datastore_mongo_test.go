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
package main_test

import (
	"errors"
	. "github.com/mendersoftware/inventory"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMongoGetDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMongoGetDevice in short mode.")
	}

	testCases := map[string]struct {
		InputID     DeviceID
		InputDevice *Device
		OutputError error
	}{
		"no device and no ID given": {
			InputID:     DeviceID(""),
			InputDevice: nil,
		},
		"device with given ID not exists": {
			InputID:     DeviceID("123"),
			InputDevice: nil,
		},
		"device with given ID exists, no error": {
			InputID: DeviceID("0002"),
			InputDevice: &Device{
				ID: DeviceID("0002"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac", Value: "0002-mac"},
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)

		// Make sure we start test with empty database
		db.Wipe()

		session := db.Session()
		store := NewDataStoreMongoWithSession(session)

		if testCase.InputDevice != nil {
			session.DB(DbName).C(DbDevicesColl).Insert(testCase.InputDevice)
		}

		dbdev, err := store.GetDevice(testCase.InputID)

		if testCase.InputDevice != nil {
			assert.NotNil(t, dbdev, "expected to device of ID %s to be found", testCase.InputDevice.ID)
			assert.Equal(t, testCase.InputID, dbdev.ID)
		} else {
			assert.Nil(t, dbdev, "expected no device to be found")
		}

		assert.NoError(t, err, "expected no error")

		// Need to close all sessions to be able to call wipe at next test case
		session.Close()
	}
}

func TestMongoAddDevice(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMongoAddDevice in short mode.")
	}

	testCases := map[string]struct {
		InputDevice *Device
		OutputError error
	}{
		"no device given": {
			InputDevice: nil,
			OutputError: errors.New("failed to store device: error parsing element 0 of field documents :: caused by :: wrong type for '0' field, expected object, found 0: null"),
		},
		"valid device with one attribute, no error": {
			InputDevice: &Device{
				ID: DeviceID("0002"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac", Value: "0002-mac"},
				},
			},
			OutputError: nil,
		},
		"valid device with two attributes, no error": {
			InputDevice: &Device{
				ID: DeviceID("0003"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac", Value: "0002-mac"},
					"sn":  DeviceAttribute{Name: "sn", Value: "0002-sn"},
				},
			},
			OutputError: nil,
		},
		"valid device with attribute without value, no error": {
			InputDevice: &Device{
				ID: DeviceID("0004"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac"},
				},
			},
			OutputError: nil,
		},
		"valid device with array in attribute value, no error": {
			InputDevice: &Device{
				ID: DeviceID("0005"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac", Value: []interface{}{123, 456}},
				},
			},
			OutputError: nil,
		},
		"valid device without attributes, no error": {
			InputDevice: &Device{
				ID: DeviceID("0007"),
				Attributes: DeviceAttributes{
					"mac": DeviceAttribute{Name: "mac"},
				},
			},
			OutputError: nil,
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)

		// Make sure we start test with empty database
		db.Wipe()

		session := db.Session()
		store := NewDataStoreMongoWithSession(session)

		err := store.AddDevice(testCase.InputDevice)

		if testCase.OutputError != nil {
			assert.EqualError(t, err, testCase.OutputError.Error())
		} else {
			assert.NoError(t, err, "expected no error inserting to data store")

			var dbdev *Device
			devsColl := session.DB(DbName).C(DbDevicesColl)
			err := devsColl.Find(nil).One(&dbdev)

			assert.NoError(t, err, "expected no error")

			assert.NotNil(t, dbdev, "expected to device of ID %s to be found", testCase.InputDevice.ID)

			assert.Equal(t, testCase.InputDevice.ID, dbdev.ID)
		}

		// Need to close all sessions to be able to call wipe at next test case
		session.Close()
	}
}

func TestMongoAddGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestMongoAddGroup in short mode.")
	}

	description := "Some description"

	testCases := map[string]struct {
		InputGroup  *Group
		OutputError error
	}{
		"no group given": {
			InputGroup:  nil,
			OutputError: errors.New("failed to store group: error parsing element 0 of field documents :: caused by :: wrong type for '0' field, expected object, found 0: null"),
		},
		"valid group , no error": {
			InputGroup: &Group{
				Name:        "Group name",
				Description: &description,
				DeviceIDs:   []DeviceID{"1", "2"},
			},
			OutputError: nil,
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)

		// Make sure we start test with empty database
		db.Wipe()

		session := db.Session()
		store := NewDataStoreMongoWithSession(session)

		err := store.AddGroup(testCase.InputGroup)

		if testCase.OutputError != nil {
			assert.EqualError(t, err, testCase.OutputError.Error())
		} else {
			assert.NoError(t, err, "expected no error inserting to data store")

			var dbgroup *Group
			groupsColl := session.DB(DbName).C(DbGroupsColl)
			err := groupsColl.Find(nil).One(&dbgroup)

			assert.NoError(t, err, "expected no error")

			assert.NotNil(t, dbgroup, "expected to group with name: %s to be found", testCase.InputGroup.Name)

			assert.Equal(t, testCase.InputGroup.Name, dbgroup.Name)
		}

		// Need to close all sessions to be able to call wipe at next test case
		session.Close()
	}
}

func TestNewDataStoreMongo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TestNewDataStoreMongo in short mode.")
	}

	ds, err := NewDataStoreMongo("illegal url")

	assert.Nil(t, ds)
	assert.EqualError(t, err, "failed to open mgo session")
}

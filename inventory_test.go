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
	"errors"
	"github.com/stretchr/testify/assert"
	. "github.com/stretchr/testify/mock"
	"testing"
)

func invForTest(d DataStore) InventoryApp {
	return &Inventory{db: d}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestInventoryListDevices(t *testing.T) {
	t.Parallel()

	group := GroupName("asd")
	testCases := map[string]struct {
		inHasGroup      *bool
		datastoreFilter []Filter
		datastoreError  error
		outError        error
		outDevices      []Device
	}{
		"has group nil": {
			inHasGroup:      nil,
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []Device{Device{ID: DeviceID("1")}},
		},
		"has group true": {
			inHasGroup:      boolPtr(true),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []Device{Device{ID: DeviceID("1"), Group: group}},
		},
		"has group false": {
			inHasGroup:      boolPtr(false),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []Device{Device{ID: DeviceID("1")}},
		},
		"datastore error": {
			inHasGroup:      nil,
			datastoreFilter: nil,
			datastoreError:  errors.New("db connection failed"),
			outError:        errors.New("failed to fetch devices: db connection failed"),
			outDevices:      nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &MockDataStore{}
		db.On("GetDevices",
			AnythingOfType("int"),
			AnythingOfType("int"),
			tc.datastoreFilter,
			AnythingOfType("*main.Sort"),
			AnythingOfType("*bool"),
		).Return(tc.outDevices, tc.datastoreError)
		i := invForTest(db)

		devs, err := i.ListDevices(1, 10, nil, nil, tc.inHasGroup)

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
			assert.Equal(t, len(devs), len(tc.outDevices))
		}
	}
}

func TestInventoryAddDevice(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDevice       *Device
		datastoreError error
		outError       error
	}{
		"nil device": {
			inDevice:       nil,
			datastoreError: nil,
			outError:       errors.New("no device given"),
		},
		"datastore success": {
			inDevice:       &Device{},
			datastoreError: nil,
			outError:       nil,
		},
		"datastore error": {
			inDevice:       &Device{},
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to add device: db connection failed"),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &MockDataStore{}
		db.On("AddDevice", AnythingOfType("*main.Device")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.AddDevice(tc.inDevice)

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestInventoryUpsertAttributes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		datastoreError error
		outError       error
	}{
		"datastore success": {
			datastoreError: nil,
			outError:       nil,
		},
		"datastore error": {
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to upsert attributes in db: db connection failed"),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &MockDataStore{}
		db.On("UpsertAttributes", AnythingOfType("main.DeviceID"), AnythingOfType("main.DeviceAttributes")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UpsertAttributes("devid", DeviceAttributes{})

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestInventoryUnsetDeviceGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDeviceID     DeviceID
		inGroupName    GroupName
		datastoreError error
		outError       error
	}{
		"empty device ID, not found": {
			inDeviceID:     DeviceID(""),
			inGroupName:    GroupName("gr1"),
			datastoreError: ErrDevNotFound,
			outError:       ErrDevNotFound,
		},
		"device group name not matching": {
			inDeviceID:     DeviceID("1"),
			inGroupName:    GroupName("not-matching"),
			datastoreError: ErrDevNotFound,
			outError:       ErrDevNotFound,
		},
		"datastore success": {
			inDeviceID:     DeviceID("1"),
			inGroupName:    GroupName("gr1"),
			datastoreError: nil,
			outError:       nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &MockDataStore{}
		db.On("UnsetDeviceGroup", AnythingOfType("main.DeviceID"), AnythingOfType("main.GroupName")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UnsetDeviceGroup(tc.inDeviceID, tc.inGroupName)

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestInventoryUpdateDeviceGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDeviceID     DeviceID
		inGroupName    GroupName
		datastoreError error
		outError       error
	}{
		"empty device ID, not found": {
			inDeviceID:     DeviceID(""),
			inGroupName:    GroupName("gr1"),
			datastoreError: ErrDevNotFound,
			outError:       errors.New("failed to add device to group: Device not found"),
		},
		"datastore success": {
			inDeviceID:     DeviceID("1"),
			inGroupName:    GroupName("gr1"),
			datastoreError: nil,
			outError:       nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &MockDataStore{}
		db.On("UpdateDeviceGroup", AnythingOfType("main.DeviceID"), AnythingOfType("main.GroupName")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UpdateDeviceGroup(tc.inDeviceID, tc.inGroupName)

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
		}
	}
}

func TestNewInventory(t *testing.T) {
	t.Parallel()

	i := NewInventory(&MockDataStore{})

	assert.NotNil(t, i)
}

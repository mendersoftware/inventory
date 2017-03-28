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
package inv

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
	mstore "github.com/mendersoftware/inventory/store/mocks"
)

func invForTest(d store.DataStore) InventoryApp {
	return &Inventory{db: d}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestInventoryListDevices(t *testing.T) {
	t.Parallel()

	group := model.GroupName("asd")
	testCases := map[string]struct {
		inHasGroup      *bool
		datastoreFilter []store.Filter
		datastoreError  error
		outError        error
		outDevices      []model.Device
	}{
		"has group nil": {
			inHasGroup:      nil,
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1")}},
		},
		"has group true": {
			inHasGroup:      boolPtr(true),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1"), Group: group}},
		},
		"has group false": {
			inHasGroup:      boolPtr(false),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1")}},
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

		db := &mstore.DataStore{}
		db.On("GetDevices",
			mock.AnythingOfType("int"),
			mock.AnythingOfType("int"),
			tc.datastoreFilter,
			mock.AnythingOfType("*store.Sort"),
			mock.AnythingOfType("*bool"),
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

func TestInventoryGetDevice(t *testing.T) {
	t.Parallel()

	group := model.GroupName("asd")
	testCases := map[string]struct {
		devid          model.DeviceID
		datastoreError error
		outError       error
		outDevice      *model.Device
	}{
		"has device": {
			devid:     model.DeviceID("1"),
			outDevice: &model.Device{ID: model.DeviceID("1"), Group: group},
		},
		"no device": {
			devid:     model.DeviceID("2"),
			outDevice: nil,
		},
		"datastore error": {
			devid:          model.DeviceID("3"),
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to fetch device: db connection failed"),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}
		db.On("GetDevice",
			mock.AnythingOfType("model.DeviceID"),
		).Return(tc.outDevice, tc.datastoreError)
		i := invForTest(db)

		dev, err := i.GetDevice(tc.devid)

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
			if tc.outDevice != nil && assert.NotNil(t, dev) {
				assert.Equal(t, *tc.outDevice, *dev)
			} else {
				assert.Nil(t, dev)
			}
		}
	}
}

func TestInventoryAddDevice(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDevice       *model.Device
		datastoreError error
		outError       error
	}{
		"nil device": {
			inDevice:       nil,
			datastoreError: nil,
			outError:       errors.New("no device given"),
		},
		"datastore success": {
			inDevice:       &model.Device{},
			datastoreError: nil,
			outError:       nil,
		},
		"datastore error": {
			inDevice:       &model.Device{},
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to add device: db connection failed"),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}
		db.On("AddDevice", mock.AnythingOfType("*model.Device")).
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

		db := &mstore.DataStore{}
		db.On("UpsertAttributes", mock.AnythingOfType("model.DeviceID"), mock.AnythingOfType("model.DeviceAttributes")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UpsertAttributes("devid", model.DeviceAttributes{})

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
		inDeviceID     model.DeviceID
		inGroupName    model.GroupName
		datastoreError error
		outError       error
	}{
		"empty device ID, not found": {
			inDeviceID:     model.DeviceID(""),
			inGroupName:    model.GroupName("gr1"),
			datastoreError: store.ErrDevNotFound,
			outError:       store.ErrDevNotFound,
		},
		"device group name not matching": {
			inDeviceID:     model.DeviceID("1"),
			inGroupName:    model.GroupName("not-matching"),
			datastoreError: store.ErrDevNotFound,
			outError:       store.ErrDevNotFound,
		},
		"datastore success": {
			inDeviceID:     model.DeviceID("1"),
			inGroupName:    model.GroupName("gr1"),
			datastoreError: nil,
			outError:       nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}
		db.On("UnsetDeviceGroup", mock.AnythingOfType("model.DeviceID"), mock.AnythingOfType("model.GroupName")).
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
		inDeviceID     model.DeviceID
		inGroupName    model.GroupName
		datastoreError error
		outError       error
	}{
		"empty device ID, not found": {
			inDeviceID:     model.DeviceID(""),
			inGroupName:    model.GroupName("gr1"),
			datastoreError: store.ErrDevNotFound,
			outError:       errors.New("failed to add device to group: Device not found"),
		},
		"datastore success": {
			inDeviceID:     model.DeviceID("1"),
			inGroupName:    model.GroupName("gr1"),
			datastoreError: nil,
			outError:       nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}
		db.On("UpdateDeviceGroup", mock.AnythingOfType("model.DeviceID"), mock.AnythingOfType("model.GroupName")).
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

func TestInventoryListGroups(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inputGroups    []model.GroupName
		datastoreError error
		outputGroups   []model.GroupName
		outError       error
	}{
		"some groups": {
			inputGroups:  []model.GroupName{"foo", "bar"},
			outputGroups: []model.GroupName{"foo", "bar"},
		},
		"no groups - nil": {
			inputGroups:  nil,
			outputGroups: []model.GroupName{},
		},
		"no groups - empty slice": {
			inputGroups:  []model.GroupName{},
			outputGroups: []model.GroupName{},
		},
		"error": {
			datastoreError: errors.New("random error"),
			outError:       errors.New("failed to list groups: random error"),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}

		db.On("ListGroups").Return(tc.inputGroups, tc.datastoreError)
		i := invForTest(db)

		groups, err := i.ListGroups()

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
			assert.EqualValues(t, tc.outputGroups, groups)
		}
	}
}

func TestInventoryListDevicesByGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		DatastoreError error
		OutError       string
		OutDevices     []model.DeviceID
	}{
		"success": {
			DatastoreError: nil,
			OutError:       "",
			OutDevices: []model.DeviceID{
				model.DeviceID("1"),
				model.DeviceID("2"),
				model.DeviceID("3"),
			},
		},
		"success - empty list": {
			DatastoreError: nil,
			OutError:       "",
			OutDevices:     []model.DeviceID{},
		},
		"datastore error - group not found": {
			DatastoreError: store.ErrGroupNotFound,
			OutError:       "group not found",
			OutDevices:     nil,
		},
		"datastore error - generic": {
			DatastoreError: errors.New("datastore error"),
			OutError:       "failed to list devices by group: datastore error",
			OutDevices:     nil,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}

		db.On("GetDevicesByGroup",
			mock.AnythingOfType("model.GroupName"),
			mock.AnythingOfType("int"),
			mock.AnythingOfType("int"),
		).Return(tc.OutDevices, tc.DatastoreError)

		i := invForTest(db)

		devs, err := i.ListDevicesByGroup("foo", 1, 1)

		if tc.OutError != "" {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.OutError)
			}
		} else {
			assert.NoError(t, err)
			if !reflect.DeepEqual(tc.OutDevices, devs) {
				assert.Fail(t, "expected: %v\nhave: %v", tc.OutDevices, devs)
			}
		}
	}
}

func TestInventoryGetDeviceGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		DatastoreError error
		DatastoreGroup model.GroupName
		OutError       error
		OutGroup       model.GroupName
	}{
		"success - device has group": {
			DatastoreError: nil,
			DatastoreGroup: model.GroupName("dev"),
			OutError:       nil,
			OutGroup:       model.GroupName("dev"),
		},
		"success - device has no group": {
			DatastoreError: nil,
			DatastoreGroup: model.GroupName(""),
			OutError:       nil,
			OutGroup:       model.GroupName(""),
		},
		"datastore error - device not found": {
			DatastoreError: store.ErrDevNotFound,
			DatastoreGroup: model.GroupName(""),
			OutError:       store.ErrDevNotFound,
			OutGroup:       model.GroupName(""),
		},
		"datastore error - generic": {
			DatastoreError: errors.New("datastore error"),
			DatastoreGroup: model.GroupName(""),
			OutError:       errors.New("failed to get device's group: datastore error"),
			OutGroup:       model.GroupName(""),
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		db := &mstore.DataStore{}

		db.On("GetDeviceGroup",
			mock.AnythingOfType("model.DeviceID"),
		).Return(tc.OutGroup, tc.DatastoreError)

		i := invForTest(db)

		group, err := i.GetDeviceGroup("foo")

		if tc.OutError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.OutError.Error())
			}
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tc.OutGroup, group)
		}
	}
}

func TestInventoryDeleteDevice(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		datastoreError error
		outError       error
	}{
		"ok": {
			datastoreError: nil,
			outError:       nil,
		},
		"no device": {
			datastoreError: store.ErrDevNotFound,
			outError:       store.ErrDevNotFound,
		},
		"datastore error": {
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to delete device: db connection failed"),
		},
	}

	for name, tc := range testCases {
		t.Run(fmt.Sprintf("test case: %s", name), func(t *testing.T) {

			db := &mstore.DataStore{}
			db.On("DeleteDevice",
				mock.AnythingOfType("DeviceID"),
			).Return(tc.datastoreError)
			i := invForTest(db)

			err := i.DeleteDevice(model.DeviceID("foo"))

			if tc.outError != nil {
				if assert.Error(t, err) {
					assert.EqualError(t, err, tc.outError.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewInventory(t *testing.T) {
	t.Parallel()

	i := NewInventory(&mstore.DataStore{})

	assert.NotNil(t, i)
}

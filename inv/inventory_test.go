// Copyright 2019 Northern.tech AS
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
	"context"
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
	return &inventory{db: d}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestInventoryListDevices(t *testing.T) {
	t.Parallel()

	group := model.GroupName("asd")
	testCases := map[string]struct {
		group           string
		inHasGroup      *bool
		datastoreFilter []store.Filter
		datastoreError  error
		outError        error
		outDevices      []model.Device
		outDeviceCount  int
	}{
		"has group nil": {
			inHasGroup:      nil,
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1")}},
			outDeviceCount:  1,
		},
		"has group true": {
			inHasGroup:      boolPtr(true),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1"), Group: group}},
			outDeviceCount:  1,
		},
		"has group false": {
			inHasGroup:      boolPtr(false),
			datastoreFilter: nil,
			datastoreError:  nil,
			outError:        nil,
			outDevices:      []model.Device{{ID: model.DeviceID("1")}},
			outDeviceCount:  1,
		},
		"datastore error": {
			inHasGroup:      nil,
			datastoreFilter: nil,
			datastoreError:  errors.New("db connection failed"),
			outError:        errors.New("failed to fetch devices: db connection failed"),
			outDevices:      nil,
			outDeviceCount:  -1,
		},
		"get devices from group": {
			group: "asd",
			outDevices: []model.Device{
				{ID: model.DeviceID("1"), Group: group},
				{ID: model.DeviceID("2"), Group: group},
			},
			outDeviceCount: 2,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("GetDevices",
			ctx,
			mock.AnythingOfType("store.ListQuery"),
		).Return(tc.outDevices, tc.outDeviceCount, tc.datastoreError)
		i := invForTest(db)

		devs, totalCount, err := i.ListDevices(ctx,
			store.ListQuery{
				Skip:      1,
				Limit:     10,
				Filters:   nil,
				Sort:      nil,
				HasGroup:  tc.inHasGroup,
				GroupName: tc.group})

		if tc.outError != nil {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.outError.Error())
			}
		} else {
			assert.NoError(t, err)
			assert.Equal(t, len(devs), len(tc.outDevices))
			assert.Equal(t, totalCount, tc.outDeviceCount)
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

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("GetDevice",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
		).Return(tc.outDevice, tc.datastoreError)
		i := invForTest(db)

		dev, err := i.GetDevice(ctx, tc.devid)

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

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("AddDevice",
			ctx,
			mock.AnythingOfType("*model.Device")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.AddDevice(ctx, tc.inDevice)

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

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("UpsertAttributes",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.DeviceAttributes")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UpsertAttributes(ctx, "devid", model.DeviceAttributes{})

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

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("UnsetDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.GroupName")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UnsetDeviceGroup(ctx, tc.inDeviceID, tc.inGroupName)

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

		ctx := context.Background()

		db := &mstore.DataStore{}
		db.On("UpdateDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.GroupName")).
			Return(tc.datastoreError)
		i := invForTest(db)

		err := i.UpdateDeviceGroup(ctx, tc.inDeviceID, tc.inGroupName)

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

		ctx := context.Background()

		db := &mstore.DataStore{}

		db.On("ListGroups", ctx).
			Return(tc.inputGroups, tc.datastoreError)
		i := invForTest(db)

		groups, err := i.ListGroups(ctx)

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
		OutDeviceCount int
	}{
		"success": {
			DatastoreError: nil,
			OutError:       "",
			OutDevices: []model.DeviceID{
				model.DeviceID("1"),
				model.DeviceID("2"),
				model.DeviceID("3"),
			},
			OutDeviceCount: 3,
		},
		"success - empty list": {
			DatastoreError: nil,
			OutError:       "",
			OutDevices:     []model.DeviceID{},
			OutDeviceCount: 0,
		},
		"datastore error - group not found": {
			DatastoreError: store.ErrGroupNotFound,
			OutError:       "group not found",
			OutDevices:     nil,
			OutDeviceCount: -1,
		},
		"datastore error - generic": {
			DatastoreError: errors.New("datastore error"),
			OutError:       "failed to list devices by group: datastore error",
			OutDevices:     nil,
			OutDeviceCount: -1,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)

		ctx := context.Background()

		db := &mstore.DataStore{}

		db.On("GetDevicesByGroup",
			ctx,
			mock.AnythingOfType("model.GroupName"),
			mock.AnythingOfType("int"),
			mock.AnythingOfType("int"),
		).Return(tc.OutDevices, tc.OutDeviceCount, tc.DatastoreError)

		i := invForTest(db)

		devs, totalCount, err := i.ListDevicesByGroup(ctx, "foo", 1, 1)

		if tc.OutError != "" {
			if assert.Error(t, err) {
				assert.EqualError(t, err, tc.OutError)
			}
		} else {
			assert.NoError(t, err)
			if !reflect.DeepEqual(tc.OutDevices, devs) {
				assert.Fail(t, "expected outDevices to match", fmt.Sprintf("Expected: %v but\n have:%v", tc.OutDevices, devs))
			}
			if !reflect.DeepEqual(tc.OutDeviceCount, totalCount) {
				assert.Fail(t, "expected outDeviceCount to match", fmt.Sprintf("Expected: %v but\n have:%v", tc.OutDeviceCount, totalCount))
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

		ctx := context.Background()

		db := &mstore.DataStore{}

		db.On("GetDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
		).Return(tc.OutGroup, tc.DatastoreError)

		i := invForTest(db)

		group, err := i.GetDeviceGroup(ctx, "foo")

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
			ctx := context.Background()

			db := &mstore.DataStore{}
			db.On("DeleteDevice",
				ctx,
				mock.AnythingOfType("DeviceID"),
			).Return(tc.datastoreError)
			i := invForTest(db)

			err := i.DeleteDevice(ctx, model.DeviceID("foo"))

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

	i := NewInventory(&mstore.DataStore{}, nil)

	assert.NotNil(t, i)
}

func TestUserAdmCreateTenant(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		tenant    string
		tenantErr error
		err       error
	}{
		"ok": {
			tenant: "foobar",
		},
		"error": {
			tenant:    "1234",
			tenantErr: errors.New("migration failed"),
			err:       errors.New("failed to apply migrations for tenant 1234: migration failed"),
		},
	}

	for name := range testCases {
		tc := testCases[name]
		t.Run(fmt.Sprintf("tc %s", name), func(t *testing.T) {

			t.Logf("test case: %s", name)

			ctx := context.Background()

			tenantDb := &mstore.TenantDataKeeper{}
			tenantDb.On("MigrateTenant", ctx, tc.tenant).Return(tc.tenantErr)

			useradm := NewInventory(nil, tenantDb)

			err := useradm.CreateTenant(ctx, model.NewTenant{ID: tc.tenant})
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

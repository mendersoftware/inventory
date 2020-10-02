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
	"github.com/mendersoftware/inventory/store/mongo"
)

func invForTest(d store.DataStore) InventoryApp {
	return &inventory{db: d}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name           string
		DataStoreError error
	}{{
		Name: "ok",
	}, {
		Name:           "error, error reaching MongoDB",
		DataStoreError: errors.New("connection refused"),
	}}
	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			ctx := context.TODO()
			db := &mstore.DataStore{}
			db.On("Ping", ctx).Return(tc.DataStoreError)
			inv := NewInventory(db)
			err := inv.HealthCheck(ctx)
			if tc.DataStoreError != nil {
				assert.EqualError(t, err,
					"error reaching MongoDB: "+
						tc.DataStoreError.Error(),
				)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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
		t.Run(name, func(t *testing.T) {
			t.Logf("test case: %s", name)

			ctx := context.Background()

			db := &mstore.DataStore{}
			db.On("UpsertDevicesAttributes",
				ctx,
				mock.AnythingOfType("[]model.DeviceID"),
				mock.AnythingOfType("model.DeviceAttributes")).
				Return(nil, tc.datastoreError)
			i := invForTest(db)

			err := i.UpsertAttributes(ctx, "devid", model.DeviceAttributes{})

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

func TestReplaceAttributes(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		deviceID       model.DeviceID
		getDevice      *model.Device
		getDeviceErr   error
		datastoreError error

		upsertAttrs model.DeviceAttributes
		removeAttrs model.DeviceAttributes
		outError    error
	}{
		"ok, device not found": {
			deviceID:     "1",
			getDevice:    nil,
			getDeviceErr: store.ErrDevNotFound,

			upsertAttrs: model.DeviceAttributes{
				model.DeviceAttribute{
					Name:  "name",
					Value: "foo",
					Scope: model.AttrScopeInventory,
				},
				model.DeviceAttribute{
					Name:  "ip_address",
					Value: "127.0.0.1",
					Scope: model.AttrScopeInventory,
				},
			},
			removeAttrs: model.DeviceAttributes{},

			datastoreError: nil,
			outError:       nil,
		},
		"ok, device found": {
			deviceID: "1",
			getDevice: &model.Device{
				Attributes: model.DeviceAttributes{},
			},
			getDeviceErr: nil,

			upsertAttrs: model.DeviceAttributes{
				model.DeviceAttribute{
					Name:  "name",
					Value: "foo",
					Scope: model.AttrScopeInventory,
				},
				model.DeviceAttribute{
					Name:  "ip_address",
					Value: "127.0.0.1",
					Scope: model.AttrScopeInventory,
				},
			},
			removeAttrs: model.DeviceAttributes{},

			datastoreError: nil,
			outError:       nil,
		},
		"ok, device found, replace attributes": {
			deviceID: "1",
			getDevice: &model.Device{
				Attributes: model.DeviceAttributes{
					model.DeviceAttribute{
						Name:  "name",
						Value: "foo",
						Scope: model.AttrScopeInventory,
					},
					model.DeviceAttribute{
						Name:  "custom",
						Value: "bar",
						Scope: model.AttrScopeInventory,
					},
				},
			},
			getDeviceErr: nil,

			upsertAttrs: model.DeviceAttributes{
				model.DeviceAttribute{
					Name:  "name",
					Value: "foo",
					Scope: model.AttrScopeInventory,
				},
				model.DeviceAttribute{
					Name:  "ip_address",
					Value: "127.0.0.1",
					Scope: model.AttrScopeInventory,
				},
			},
			removeAttrs: model.DeviceAttributes{
				model.DeviceAttribute{
					Name:  "custom",
					Value: "bar",
					Scope: model.AttrScopeInventory,
				},
			},

			datastoreError: nil,
			outError:       nil,
		},
		"ko, get device error": {
			deviceID:     "1",
			getDevice:    nil,
			getDeviceErr: errors.New("get device error"),

			datastoreError: nil,
			outError:       errors.New("failed to get the device: get device error"),
		},
		"ko, datastore error": {
			deviceID:     "1",
			getDevice:    nil,
			getDeviceErr: store.ErrDevNotFound,

			upsertAttrs: model.DeviceAttributes{
				model.DeviceAttribute{
					Name:  "name",
					Value: "foo",
					Scope: model.AttrScopeInventory,
				},
				model.DeviceAttribute{
					Name:  "ip_address",
					Value: "127.0.0.1",
					Scope: model.AttrScopeInventory,
				},
			},
			removeAttrs: model.DeviceAttributes{},

			datastoreError: errors.New("get device error"),
			outError:       errors.New("failed to replace attributes in db: get device error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			db := &mstore.DataStore{}
			defer db.AssertExpectations(t)

			db.On("GetDevice",
				ctx,
				tc.deviceID,
			).Return(tc.getDevice, tc.getDeviceErr)

			if tc.getDeviceErr == nil || tc.getDeviceErr == store.ErrDevNotFound {
				db.On("UpsertRemoveDeviceAttributes",
					ctx,
					tc.deviceID,
					tc.upsertAttrs,
					tc.removeAttrs,
				).Return(nil, tc.datastoreError)
			}

			i := invForTest(db)
			err := i.ReplaceAttributes(ctx, tc.deviceID, tc.upsertAttrs, model.AttrScopeInventory)

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

func TestInventoryUnsetDeviceGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDeviceID      model.DeviceID
		inGroupName     model.GroupName
		datastoreError  error
		datastoreResult *model.UpdateResult
		outError        error
	}{
		"empty device ID, not found": {
			inDeviceID:  model.DeviceID(""),
			inGroupName: model.GroupName("gr1"),
			datastoreResult: &model.UpdateResult{
				MatchedCount: 0,
				UpdatedCount: 0,
			},
			outError: store.ErrDevNotFound,
		},
		"device group name not matching": {
			inDeviceID:  model.DeviceID("1"),
			inGroupName: model.GroupName("not-matching"),
			datastoreResult: &model.UpdateResult{
				MatchedCount: 0,
				UpdatedCount: 0,
			},
			outError: store.ErrDevNotFound,
		},
		"datastore success": {
			inDeviceID:  model.DeviceID("1"),
			inGroupName: model.GroupName("gr1"),
			datastoreResult: &model.UpdateResult{
				MatchedCount: 1,
				UpdatedCount: 1,
			},
			datastoreError: nil,
			outError:       nil,
		},
		"datastore internal error": {
			inDeviceID:      model.DeviceID("1"),
			inGroupName:     model.GroupName("gr1"),
			datastoreResult: nil,
			datastoreError:  errors.New("internal error"),
			outError:        errors.New("failed to unassign group from device: internal error"),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			db := &mstore.DataStore{}
			db.On("UnsetDevicesGroup",
				ctx,
				mock.AnythingOfType("[]model.DeviceID"),
				mock.AnythingOfType("model.GroupName")).
				Return(tc.datastoreResult, tc.datastoreError)
			i := invForTest(db)

			err := i.UnsetDeviceGroup(ctx, tc.inDeviceID, tc.inGroupName)

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

func TestInventoryUpdateDeviceGroup(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inDeviceID      model.DeviceID
		inGroupName     model.GroupName
		datastoreResult *model.UpdateResult
		datastoreError  error
		outError        error
	}{
		"empty device ID, not found": {
			inDeviceID:      model.DeviceID(""),
			inGroupName:     model.GroupName("gr1"),
			datastoreResult: &model.UpdateResult{},
			outError:        errors.New("Device not found"),
		},
		"datastore success": {
			inDeviceID:  model.DeviceID("1"),
			inGroupName: model.GroupName("gr1"),
			datastoreResult: &model.UpdateResult{
				MatchedCount: 1,
				UpdatedCount: 1,
			},
			datastoreError: nil,
			outError:       nil,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Logf("test case: %s", name)

			ctx := context.Background()

			db := &mstore.DataStore{}
			db.On("UpdateDevicesGroup",
				ctx,
				mock.AnythingOfType("[]model.DeviceID"),
				mock.AnythingOfType("model.GroupName")).
				Return(tc.datastoreResult, tc.datastoreError)
			i := invForTest(db)

			err := i.UpdateDeviceGroup(ctx, tc.inDeviceID, tc.inGroupName)

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

func TestInventoryListGroups(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inputGroups    []model.GroupName
		outputGroups   []model.GroupName
		filters        []model.FilterPredicate
		datastoreError error
		outError       error
	}{
		"some groups": {
			inputGroups:  []model.GroupName{"foo", "bar"},
			outputGroups: []model.GroupName{"foo", "bar"},
		},
		"no groups - nil": {
			inputGroups:  nil,
			outputGroups: []model.GroupName{},
			filters: []model.FilterPredicate{{
				Attribute: "status",
				Scope:     model.AttrScopeIdentity,
				Type:      "$eq",
				Value:     "rejected",
			}},
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
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			db := &mstore.DataStore{}

			db.On("ListGroups", ctx, tc.filters).
				Return(tc.inputGroups, tc.datastoreError)
			i := invForTest(db)

			groups, err := i.ListGroups(ctx, tc.filters)
			if tc.outError != nil {
				if assert.Error(t, err) {
					assert.EqualError(t, err, tc.outError.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.EqualValues(t, tc.outputGroups, groups)
			}
		})
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
		datastoreResult *model.UpdateResult
		datastoreError  error
		outError        error
	}{
		"ok": {
			datastoreResult: &model.UpdateResult{
				DeletedCount: 1,
			},
			outError: nil,
		},
		"no device": {
			datastoreResult: &model.UpdateResult{
				DeletedCount: 0,
			},
			outError: store.ErrDevNotFound,
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
			db.On("DeleteDevices",
				ctx,
				mock.AnythingOfType("[]model.DeviceID"),
			).Return(tc.datastoreResult, tc.datastoreError)
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

	i := NewInventory(&mstore.DataStore{})

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

			tenantDb := &mstore.DataStore{}
			tenantDb.On("MigrateTenant",
				ctx, mongo.DbVersion, tc.tenant).
				Return(tc.tenantErr)
			tenantDb.On("WithAutomigrate").Return(tenantDb)

			useradm := NewInventory(tenantDb)

			err := useradm.CreateTenant(ctx, model.NewTenant{ID: tc.tenant})
			if tc.err != nil {
				assert.EqualError(t, err, tc.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestInventorySearchDevices(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		searchParams   model.SearchParams
		datastoreError error
		outError       error
		outDevices     []model.Device
		outDeviceCount int
	}{
		"ok": {
			searchParams:   model.SearchParams{},
			datastoreError: nil,
			outError:       nil,
			outDevices:     []model.Device{{ID: model.DeviceID("1")}},
			outDeviceCount: 1,
		},
		"datastore error": {
			searchParams:   model.SearchParams{},
			datastoreError: errors.New("db connection failed"),
			outError:       errors.New("failed to fetch devices: db connection failed"),
			outDevices:     nil,
			outDeviceCount: -1,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			db := &mstore.DataStore{}
			db.On("SearchDevices",
				ctx,
				mock.AnythingOfType("model.SearchParams"),
			).Return(tc.outDevices, tc.outDeviceCount, tc.datastoreError)
			i := invForTest(db)

			devs, totalCount, err := i.SearchDevices(ctx, tc.searchParams)

			if tc.outError != nil {
				if assert.Error(t, err) {
					assert.EqualError(t, err, tc.outError.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(devs), len(tc.outDevices))
				assert.Equal(t, totalCount, tc.outDeviceCount)
			}
		})
	}
}

func TestInventoryUpdateDevicesGroup(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		DeviceIDs []model.DeviceID
		model.GroupName
		*model.UpdateResult
		Err error
	}{{
		Name: "ok",
		DeviceIDs: []model.DeviceID{
			"1", "2", "3",
		},
		GroupName: "foo",
		UpdateResult: &model.UpdateResult{
			UpdatedCount: 2,
			MatchedCount: 2,
		},
		Err: nil,
	}, {
		Name: "datastore error",
		DeviceIDs: []model.DeviceID{
			"1", "2", "3",
		},
		GroupName: "bar",
		Err:       errors.New("doesn't matter"),
	}}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			db := &mstore.DataStore{}
			db.On("UpdateDevicesGroup",
				ctx,
				testCase.DeviceIDs,
				testCase.GroupName,
			).Return(
				testCase.UpdateResult,
				testCase.Err,
			)
			i := invForTest(db)
			rsp, err := i.UpdateDevicesGroup(
				ctx, testCase.DeviceIDs, testCase.GroupName,
			)
			assert.Equal(t, testCase.UpdateResult, rsp)
			assert.Equal(t, testCase.Err, err)
		})
	}
}

func TestInventoryUnsetDevicesGroup(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		Name string

		DeviceIDs []model.DeviceID
		model.GroupName
		*model.UpdateResult
		Err error
	}{{
		Name: "ok",
		DeviceIDs: []model.DeviceID{
			"1", "2", "3",
		},
		GroupName: "foo",
		UpdateResult: &model.UpdateResult{
			MatchedCount: 2,
			UpdatedCount: 2,
		},
		Err: nil,
	}, {
		Name: "datastore error",
		DeviceIDs: []model.DeviceID{
			"1", "2", "3",
		},
		GroupName: "bar",
		Err:       errors.New("doesn't matter"),
	}}
	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			ctx := context.Background()
			db := &mstore.DataStore{}
			db.On("UnsetDevicesGroup",
				ctx,
				testCase.DeviceIDs,
				testCase.GroupName,
			).Return(
				testCase.UpdateResult,
				testCase.Err,
			)
			i := invForTest(db)
			rsp, err := i.UnsetDevicesGroup(
				ctx, testCase.DeviceIDs, testCase.GroupName,
			)
			assert.Equal(t, testCase.UpdateResult, rsp)
			assert.Equal(t, testCase.Err, err)
		})
	}
}

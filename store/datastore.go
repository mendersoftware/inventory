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

package store

import (
	"context"
	"errors"

	"github.com/mendersoftware/inventory/model"
)

var (
	// device not found
	ErrDevNotFound = errors.New("Device not found")

	ErrGroupNotFound = errors.New("group not found")
)

type DataStore interface {
	GetDevices(ctx context.Context, q ListQuery) ([]model.Device, int, error)

	// find a device with given `id`, returns the device or nil,
	// if device was not found, error and returned device are nil
	GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error)

	// insert device into data store
	//
	// ds.AddDevice(&model.Device{
	// 	ID: "foo",
	// 	Attributes: map[string]string{"token": "123"),
	// })
	AddDevice(ctx context.Context, dev *model.Device) error

	// delete device and all attributes
	DeleteDevice(ctx context.Context, id model.DeviceID) error

	// Updates the device attributes in a differential manner.
	// Nonexistent attributes are created, existing are overwritten; the device resource is also created if necessary.
	UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error

	// Unset group in device with `id`
	UnsetDeviceGroup(ctx context.Context, id model.DeviceID, groupName model.GroupName) error

	// Updates device group
	UpdateDeviceGroup(ctx context.Context, devid model.DeviceID, group model.GroupName) error

	// List groups
	ListGroups(ctx context.Context) ([]model.GroupName, error)

	// Lists devices belonging to a group
	GetDevicesByGroup(ctx context.Context, group model.GroupName, skip, limit int) ([]model.DeviceID, int, error)

	// Get device's group
	GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error)
}

// TenantDataKeeper is an interface for executing administrative operations on
// tenants
type TenantDataKeeper interface {
	// MigrateTenant migrates given tenant to the latest DB version
	MigrateTenant(ctx context.Context, id string) error
}

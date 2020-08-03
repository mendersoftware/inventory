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

	"github.com/pkg/errors"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
	"github.com/mendersoftware/inventory/store/mongo"
)

// this inventory service interface
type InventoryApp interface {
	HealthCheck(ctx context.Context) error
	ListDevices(ctx context.Context, q store.ListQuery) ([]model.Device, int, error)
	GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error)
	AddDevice(ctx context.Context, d *model.Device) error
	UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error
	UpsertDevicesAttributes(
		ctx context.Context,
		ids []model.DeviceID,
		attrs model.DeviceAttributes,
	) (*model.UpdateResult, error)
	UnsetDeviceGroup(ctx context.Context, id model.DeviceID, groupName model.GroupName) error
	UnsetDevicesGroup(
		ctx context.Context,
		deviceIDs []model.DeviceID,
		groupName model.GroupName,
	) (*model.UpdateResult, error)
	UpdateDeviceGroup(ctx context.Context, id model.DeviceID, group model.GroupName) error
	UpdateDevicesGroup(
		ctx context.Context,
		ids []model.DeviceID,
		group model.GroupName,
	) (*model.UpdateResult, error)
	ListGroups(ctx context.Context) ([]model.GroupName, error)
	ListDevicesByGroup(ctx context.Context, group model.GroupName, skip int, limit int) ([]model.DeviceID, int, error)
	GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error)
	DeleteDevice(ctx context.Context, id model.DeviceID) error
	DeleteDevices(
		ctx context.Context,
		ids []model.DeviceID,
	) (*model.UpdateResult, error)
	CreateTenant(ctx context.Context, tenant model.NewTenant) error
	SearchDevices(ctx context.Context, searchParams model.SearchParams) ([]model.Device, int, error)
}

type inventory struct {
	db store.DataStore
}

func NewInventory(d store.DataStore) InventoryApp {
	return &inventory{db: d}
}

func (i *inventory) HealthCheck(ctx context.Context) error {
	err := i.db.Ping(ctx)
	if err != nil {
		return errors.Wrap(err, "error reaching MongoDB")
	}
	return nil
}

func (i *inventory) ListDevices(ctx context.Context, q store.ListQuery) ([]model.Device, int, error) {
	devs, totalCount, err := i.db.GetDevices(ctx, q)

	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to fetch devices")
	}

	return devs, totalCount, nil
}

func (i *inventory) GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error) {
	dev, err := i.db.GetDevice(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch device")
	}
	return dev, nil
}

func (i *inventory) AddDevice(ctx context.Context, dev *model.Device) error {
	if dev == nil {
		return errors.New("no device given")
	}
	err := i.db.AddDevice(ctx, dev)
	if err != nil {
		return errors.Wrap(err, "failed to add device")
	}
	return nil
}

func (i *inventory) DeleteDevices(
	ctx context.Context,
	ids []model.DeviceID,
) (*model.UpdateResult, error) {
	return i.db.DeleteDevices(ctx, ids)
}

func (i *inventory) DeleteDevice(ctx context.Context, id model.DeviceID) error {
	res, err := i.db.DeleteDevices(ctx, []model.DeviceID{id})
	if err != nil {
		return errors.Wrap(err, "failed to delete device")
	} else if res.DeletedCount < 1 {
		return store.ErrDevNotFound
	}
	return nil
}

func (i *inventory) UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error {
	if _, err := i.db.UpsertDevicesAttributes(
		ctx, []model.DeviceID{id}, attrs,
	); err != nil {
		return errors.Wrap(err, "failed to upsert attributes in db")
	}
	return nil
}
func (i *inventory) UpsertDevicesAttributes(
	ctx context.Context,
	ids []model.DeviceID,
	attrs model.DeviceAttributes,
) (*model.UpdateResult, error) {
	return i.db.UpsertDevicesAttributes(ctx, ids, attrs)
}

func (i *inventory) UnsetDevicesGroup(
	ctx context.Context,
	deviceIDs []model.DeviceID,
	groupName model.GroupName,
) (*model.UpdateResult, error) {
	return i.db.UnsetDevicesGroup(ctx, deviceIDs, groupName)
}

func (i *inventory) UnsetDeviceGroup(ctx context.Context, id model.DeviceID, group model.GroupName) error {
	result, err := i.db.UnsetDevicesGroup(ctx, []model.DeviceID{id}, group)
	if err != nil {
		return errors.Wrap(err, "failed to unassign group from device")
	} else if result.MatchedCount <= 0 {
		return store.ErrDevNotFound
	}
	return nil
}

func (i *inventory) UpdateDevicesGroup(
	ctx context.Context,
	deviceIDs []model.DeviceID,
	group model.GroupName,
) (*model.UpdateResult, error) {
	return i.db.UpdateDevicesGroup(ctx, deviceIDs, group)
}

func (i *inventory) UpdateDeviceGroup(
	ctx context.Context,
	devid model.DeviceID,
	group model.GroupName,
) error {
	result, err := i.db.UpdateDevicesGroup(
		ctx, []model.DeviceID{devid}, group,
	)
	if err != nil {
		return errors.Wrap(err, "failed to add device to group")
	} else if result.MatchedCount <= 0 {
		return store.ErrDevNotFound
	}
	return nil
}

func (i *inventory) ListGroups(ctx context.Context) ([]model.GroupName, error) {
	groups, err := i.db.ListGroups(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list groups")
	}

	if groups == nil {
		return []model.GroupName{}, nil
	}
	return groups, nil
}

func (i *inventory) ListDevicesByGroup(ctx context.Context, group model.GroupName, skip, limit int) ([]model.DeviceID, int, error) {
	ids, totalCount, err := i.db.GetDevicesByGroup(ctx, group, skip, limit)
	if err != nil {
		if err == store.ErrGroupNotFound {
			return nil, -1, err
		} else {
			return nil, -1, errors.Wrap(err, "failed to list devices by group")
		}
	}

	return ids, totalCount, nil
}

func (i *inventory) GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error) {
	group, err := i.db.GetDeviceGroup(ctx, id)
	if err != nil {
		if err == store.ErrDevNotFound {
			return "", err
		} else {
			return "", errors.Wrap(err, "failed to get device's group")
		}
	}

	return group, nil
}

func (i *inventory) CreateTenant(ctx context.Context, tenant model.NewTenant) error {
	if err := i.db.WithAutomigrate().
		MigrateTenant(ctx, mongo.DbVersion, tenant.ID); err != nil {
		return errors.Wrapf(err, "failed to apply migrations for tenant %v", tenant.ID)
	}
	return nil
}

func (i *inventory) SearchDevices(ctx context.Context, searchParams model.SearchParams) ([]model.Device, int, error) {
	devs, totalCount, err := i.db.SearchDevices(ctx, searchParams)

	if err != nil {
		return nil, -1, errors.Wrap(err, "failed to fetch devices")
	}

	return devs, totalCount, nil
}

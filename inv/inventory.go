// Copyright 2017 Northern.tech AS
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
	"time"

	"github.com/pkg/errors"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
)

// this inventory service interface
type InventoryApp interface {
	ListDevices(ctx context.Context, skip int, limit int, filters []store.Filter, sort *store.Sort, hasGroup *bool) ([]model.Device, error)
	GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error)
	AddDevice(ctx context.Context, d *model.Device) error
	UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error
	UnsetDeviceGroup(ctx context.Context, id model.DeviceID, groupName model.GroupName) error
	UpdateDeviceGroup(ctx context.Context, id model.DeviceID, group model.GroupName) error
	ListGroups(ctx context.Context) ([]model.GroupName, error)
	ListDevicesByGroup(ctx context.Context, group model.GroupName, skip int, limit int) ([]model.DeviceID, error)
	GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error)
	DeleteDevice(ctx context.Context, id model.DeviceID) error
}

type Inventory struct {
	db store.DataStore
}

func NewInventory(d store.DataStore) *Inventory {
	return &Inventory{db: d}
}

func (i *Inventory) ListDevices(ctx context.Context, skip int, limit int, filters []store.Filter, sort *store.Sort, hasGroup *bool) ([]model.Device, error) {
	devs, err := i.db.GetDevices(ctx, skip, limit, filters, sort, hasGroup)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch devices")
	}

	return devs, nil
}

func (i *Inventory) GetDevice(ctx context.Context, id model.DeviceID) (*model.Device, error) {
	dev, err := i.db.GetDevice(ctx, id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch device")
	}
	return dev, nil
}

func (i *Inventory) AddDevice(ctx context.Context, dev *model.Device) error {
	if dev == nil {
		return errors.New("no device given")
	}
	now := time.Now()
	dev.CreatedTs = now
	dev.UpdatedTs = now
	err := i.db.AddDevice(ctx, dev)
	if err != nil {
		return errors.Wrap(err, "failed to add device")
	}
	return nil
}

func (i *Inventory) DeleteDevice(ctx context.Context, id model.DeviceID) error {
	err := i.db.DeleteDevice(ctx, id)
	switch err {
	case nil:
		return nil
	case store.ErrDevNotFound:
		return store.ErrDevNotFound
	default:
		return errors.Wrap(err, "failed to delete device")
	}
}

func (i *Inventory) UpsertAttributes(ctx context.Context, id model.DeviceID, attrs model.DeviceAttributes) error {
	if err := i.db.UpsertAttributes(ctx, id, attrs); err != nil {
		return errors.Wrap(err, "failed to upsert attributes in db")
	}

	return nil
}

func (i *Inventory) UnsetDeviceGroup(ctx context.Context, id model.DeviceID, groupName model.GroupName) error {
	err := i.db.UnsetDeviceGroup(ctx, id, groupName)
	if err != nil {
		if err.Error() == store.ErrDevNotFound.Error() {
			return err
		}
		return errors.Wrap(err, "failed to unassign group from device")
	}
	return nil
}

func (i *Inventory) UpdateDeviceGroup(ctx context.Context, devid model.DeviceID, group model.GroupName) error {
	err := i.db.UpdateDeviceGroup(ctx, devid, group)
	if err != nil {
		return errors.Wrap(err, "failed to add device to group")
	}
	return nil
}

func (i *Inventory) ListGroups(ctx context.Context) ([]model.GroupName, error) {
	groups, err := i.db.ListGroups(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list groups")
	}

	if groups == nil {
		return []model.GroupName{}, nil
	}
	return groups, nil
}

func (i *Inventory) ListDevicesByGroup(ctx context.Context, group model.GroupName, skip, limit int) ([]model.DeviceID, error) {
	ids, err := i.db.GetDevicesByGroup(ctx, group, skip, limit)
	if err != nil {
		if err == store.ErrGroupNotFound {
			return nil, err
		} else {
			return nil, errors.Wrap(err, "failed to list devices by group")
		}
	}

	return ids, nil
}

func (i *Inventory) GetDeviceGroup(ctx context.Context, id model.DeviceID) (model.GroupName, error) {
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

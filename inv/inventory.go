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
	"time"

	"github.com/pkg/errors"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
)

// this inventory service interface
type InventoryApp interface {
	ListDevices(skip int, limit int, filters []store.Filter, sort *store.Sort, hasGroup *bool) ([]model.Device, error)
	GetDevice(id model.DeviceID) (*model.Device, error)
	AddDevice(d *model.Device) error
	UpsertAttributes(id model.DeviceID, attrs model.DeviceAttributes) error
	UnsetDeviceGroup(id model.DeviceID, groupName model.GroupName) error
	UpdateDeviceGroup(id model.DeviceID, group model.GroupName) error
	ListGroups() ([]model.GroupName, error)
	ListDevicesByGroup(group model.GroupName, skip int, limit int) ([]model.DeviceID, error)
	GetDeviceGroup(id model.DeviceID) (model.GroupName, error)
	DeleteDevice(id model.DeviceID) error
}

type Inventory struct {
	db store.DataStore
}

func NewInventory(d store.DataStore) *Inventory {
	return &Inventory{db: d}
}

func (i *Inventory) ListDevices(skip int, limit int, filters []store.Filter, sort *store.Sort, hasGroup *bool) ([]model.Device, error) {
	devs, err := i.db.GetDevices(skip, limit, filters, sort, hasGroup)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch devices")
	}

	return devs, nil
}

func (i *Inventory) GetDevice(id model.DeviceID) (*model.Device, error) {
	dev, err := i.db.GetDevice(id)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch device")
	}
	return dev, nil
}

func (i *Inventory) AddDevice(dev *model.Device) error {
	if dev == nil {
		return errors.New("no device given")
	}
	now := time.Now()
	dev.CreatedTs = now
	dev.UpdatedTs = now
	err := i.db.AddDevice(dev)
	if err != nil {
		return errors.Wrap(err, "failed to add device")
	}
	return nil
}

func (i *Inventory) DeleteDevice(id model.DeviceID) error {
	err := i.db.DeleteDevice(id)
	switch err {
	case nil:
		return nil
	case store.ErrDevNotFound:
		return store.ErrDevNotFound
	default:
		return errors.Wrap(err, "failed to delete device")
	}
}

func (i *Inventory) UpsertAttributes(id model.DeviceID, attrs model.DeviceAttributes) error {
	if err := i.db.UpsertAttributes(id, attrs); err != nil {
		return errors.Wrap(err, "failed to upsert attributes in db")
	}

	return nil
}

func (i *Inventory) UnsetDeviceGroup(id model.DeviceID, groupName model.GroupName) error {
	err := i.db.UnsetDeviceGroup(id, groupName)
	if err != nil {
		if err.Error() == store.ErrDevNotFound.Error() {
			return err
		}
		return errors.Wrap(err, "failed to unassign group from device")
	}
	return nil
}

func (i *Inventory) UpdateDeviceGroup(devid model.DeviceID, group model.GroupName) error {
	err := i.db.UpdateDeviceGroup(devid, group)
	if err != nil {
		return errors.Wrap(err, "failed to add device to group")
	}
	return nil
}

func (i *Inventory) ListGroups() ([]model.GroupName, error) {
	groups, err := i.db.ListGroups()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list groups")
	}

	if groups == nil {
		return []model.GroupName{}, nil
	}
	return groups, nil
}

func (i *Inventory) ListDevicesByGroup(group model.GroupName, skip, limit int) ([]model.DeviceID, error) {
	ids, err := i.db.GetDevicesByGroup(group, skip, limit)
	if err != nil {
		if err == store.ErrGroupNotFound {
			return nil, err
		} else {
			return nil, errors.Wrap(err, "failed to list devices by group")
		}
	}

	return ids, nil
}

func (i *Inventory) GetDeviceGroup(id model.DeviceID) (model.GroupName, error) {
	group, err := i.db.GetDeviceGroup(id)
	if err != nil {
		if err == store.ErrDevNotFound {
			return "", err
		} else {
			return "", errors.Wrap(err, "failed to get device's group")
		}
	}

	return group, nil
}

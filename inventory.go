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
	"github.com/mendersoftware/inventory/config"
	"github.com/mendersoftware/inventory/log"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	"time"
)

// this inventory service interface
type InventoryApp interface {
	AddDevice(d *Device) error
	AddGroup(g *Group) (GroupID, error)
}

type Inventory struct {
	db DataStore
}

func NewInventory(d DataStore) *Inventory {
	return &Inventory{db: d}
}

func GetInventory(c config.Reader, l *log.Logger) (InventoryApp, error) {
	d, err := NewDataStoreMongo(c.GetString(SettingDb))
	if err != nil {
		return nil, errors.Wrap(err, "database connection failed")
	}

	inv := NewInventory(d)
	return inv, nil
}

func (i *Inventory) AddGroup(group *Group) (GroupID, error) {
	if group == nil {
		return "", errors.New("no group given")
	}
	group.ID = GroupID(uuid.NewV4().String())
	err := i.db.AddGroup(group)
	if err != nil {
		return "", errors.Wrap(err, "failed to add group")
	}
	return group.ID, nil
}

func createGroup(group *Group) *Group {
	return &Group{
		ID:          GroupID(uuid.NewV4().String()),
		Name:        group.Name,
		Description: group.Description,
		DeviceIDs:   group.DeviceIDs,
	}
}

func (i *Inventory) AddDevice(dev *Device) error {
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

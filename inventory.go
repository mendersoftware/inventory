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
	"time"
)

// this inventory service interface
type InventoryApp interface {
	AddDevice(d *Device) error
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
	// TODO inv .UserLog(l)
	return inv, nil
}

func (i *Inventory) AddDevice(dev *Device) error {
	dd := createDeviceDb(dev)
	err := i.db.AddDevice(dd)
	if err != nil {
		return errors.Wrap(err, "failed to add device")
	}
	return nil
}

func createDeviceDb(dev *Device) *DeviceDb {
	now := time.Now()
	return &DeviceDb{
		ID:         dev.ID,
		Attributes: createDbAttributes(dev.Attributes),
		CreatedTs:  now,
		UpdatedTs:  now,
	}
}

func createDbAttributes(attributes []DeviceAttribute) DeviceDbAttributes {
	dbAttrs := make(DeviceDbAttributes)
	for _, attr := range attributes {
		dbAttrs[attr.Name] = attr
	}
	return dbAttrs
}

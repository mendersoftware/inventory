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
	"testing"
)

//mock db with interface methods as fields
//allows monkey patching the methods without
//redefining the struct for each case
type TestDataStore struct {
	MockAddDevice func(dev *DeviceDb) error
	MockGetDevice func(id DeviceID) (*DeviceDb, error)
}

func (ds *TestDataStore) AddDevice(dev *DeviceDb) error {
	return ds.MockAddDevice(dev)
}

func (ds *TestDataStore) GetDevice(id DeviceID) (*DeviceDb, error) {
	return ds.MockGetDevice(id)
}

func addDevice(dev *DeviceDb) error {
	return nil
}

func addDeviceErr(dev *DeviceDb) error {
	return errors.New("db connection failed")
}

func invForTest(d DataStore) InventoryApp {
	return &Inventory{db: d}
}

func TestInventoryAddDevice(t *testing.T) {
	t.Parallel()

	db := &TestDataStore{
		MockAddDevice: addDevice,
	}
	i := invForTest(db)

	err := i.AddDevice(&Device{})

	assert.NoError(t, err)
}

func TestInventoryAddDeviceErr(t *testing.T) {
	t.Parallel()

	db := &TestDataStore{
		MockAddDevice: addDeviceErr,
	}
	i := invForTest(db)

	err := i.AddDevice(&Device{})

	if assert.Error(t, err) {
		assert.EqualError(t, err, "failed to add device: db connection failed")
	}
}

func TestNewInventory(t *testing.T) {
	t.Parallel()
	
	i := NewInventory(&TestDataStore{})

	assert.NotNil(t, i)
}

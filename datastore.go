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
)

var (
	ErrDuplicatedDeviceId = errors.New("Duplicated device id")
	// device not found
	ErrDevNotFound = errors.New("Device not found")
)

type DataStore interface {
	GetDevices(skip int, limit int, filters []Filter, sort *Sort, hasGroup *bool) ([]Device, error)

	// find a device with given `id`, returns the device or nil,
	// if device was not found, error and returned device are nil
	GetDevice(id DeviceID) (*Device, error)

	// insert device into data store
	//
	// ds.AddDevice(&Device{
	// 	ID: "foo",
	// 	Attributes: map[string]string{"token": "123"),
	// })
	AddDevice(dev *Device) error

	// Updates the device attributes in a differential manner.
	// Nonexistent attributes are created, existing are overwritten; the device resource is also created if necessary.
	UpsertAttributes(id DeviceID, attrs DeviceAttributes) error

	// Unset group in device with `id`
	UnsetDeviceGroup(id DeviceID, groupName GroupName) error

	// Updates device group
	UpdateDeviceGroup(devid DeviceID, group GroupName) error

	// List groups
	ListGroups() ([]GroupName, error)
}

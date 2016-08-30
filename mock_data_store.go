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

import "github.com/stretchr/testify/mock"

// MockDataStore is an autogenerated mock type for the DataStore type
type MockDataStore struct {
	mock.Mock
}

// AddDevice provides a mock function with given fields: dev
func (_m *MockDataStore) AddDevice(dev *Device) error {
	ret := _m.Called(dev)

	var r0 error
	if rf, ok := ret.Get(0).(func(*Device) error); ok {
		r0 = rf(dev)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// GetDevice provides a mock function with given fields: id
func (_m *MockDataStore) GetDevice(id DeviceID) (*Device, error) {
	ret := _m.Called(id)

	var r0 *Device
	if rf, ok := ret.Get(0).(func(DeviceID) *Device); ok {
		r0 = rf(id)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*Device)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(DeviceID) error); ok {
		r1 = rf(id)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// AddGroup provides a mock function with given fields: group
func (_m *MockDataStore) AddGroup(group *Group) error {
	ret := _m.Called(group)

	var r0 error
	if rf, ok := ret.Get(0).(func(*Group) error); ok {
		r0 = rf(group)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

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
	"time"
	"encoding/json"
)

type DeviceID string

//TODO move to model_group.go
type GroupID string

type DeviceAttribute struct {
	Name        string      `json:"name" bson:",omitempty"`
	Description *string     `json:"description" bson:",omitempty"`
	Value       interface{} `json:"value" bson:",omitempty"`
}

// Device wrapper
type Device struct {
	//system-generated device ID
	ID DeviceID `json:"id" bson:"_id,omitempty"`

	//a map of attributes names and their values.
	Attributes DeviceAttributes `json:"attributes" bson:",omitempty"`

	//device's group id
	Group *GroupID `json:"group" bson:",omitempty"`

	CreatedTs time.Time `json:"created_ts" bson:"created_ts,omitempty"`
	//Timestamp of the last attribute update.
	UpdatedTs time.Time `json:"updated_ts" bson:"updated_ts,omitempty"`
}

func (did DeviceID) String() string {
	return string(did)
}

// wrapper for device attributes names and values
type DeviceAttributes map[string]DeviceAttribute

func (d *DeviceAttributes) UnmarshalJSON(b []byte) error {
	var attrsArray []DeviceAttribute
	err := json.Unmarshal(b, &attrsArray)
	if err != nil {
		return err
	}
	if len(attrsArray) > 0 {
		*d = DeviceAttributes{}
		for _, attr := range attrsArray {
			(*d)[attr.Name] = attr
		}
	}
	return nil
}

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
package model

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/go-ozzo/ozzo-validation"
)

type DeviceID string

type GroupName string

type DeviceAttribute struct {
	Name        string      `json:"name" bson:",omitempty"`
	Description *string     `json:"description,omitempty" bson:",omitempty"`
	Value       interface{} `json:"value" bson:",omitempty"`
}

func (da DeviceAttribute) Validate() error {
	return validation.ValidateStruct(&da,
		validation.Field(&da.Name, validation.Required, validation.Length(1, 1024)),
		validation.Field(&da.Value, validation.Required, validation.By(validateDeviceAttrVal)),
	)
}

func validateDeviceAttrVal(i interface{}) error {
	switch v := i.(type) {
	case float64, string:
		return nil
	case []interface{}:
		return validateDeviceAttrValArray(v)
	default:
		return errors.New("supported types are string, float64, and arrays thereof")
	}
}

func validateDeviceAttrValArray(arr []interface{}) error {
	var firstValueString, firstValueFloat64 bool
	for i, v := range arr {
		_, isstring := v.(string)
		_, isfloat64 := v.(float64)
		if i == 0 {
			if isstring {
				firstValueString = true
			} else if isfloat64 {
				firstValueFloat64 = true
			} else {
				return errors.New("array values must be either string or float64")
			}
		} else if (firstValueString && !isstring) || (firstValueFloat64 && !isfloat64) {
			return errors.New("array values must be of consistent type (string or float64)")
		}
	}
	return nil
}

// Device wrapper
type Device struct {
	//system-generated device ID
	ID DeviceID `json:"id" bson:"_id,omitempty"`

	//a map of attributes names and their values.
	Attributes DeviceAttributes `json:"attributes,omitempty" bson:",omitempty"`

	//device's group name
	Group GroupName `json:"-" bson:"group,omitempty"`

	CreatedTs time.Time `json:"-" bson:"created_ts,omitempty"`
	//Timestamp of the last attribute update.
	UpdatedTs time.Time `json:"updated_ts" bson:"updated_ts,omitempty"`
}

func (d Device) Validate() error {
	return validation.ValidateStruct(&d,
		validation.Field(&d.ID, validation.Required, validation.Length(1, 1024)),
		validation.Field(&d.Attributes),
	)
}

func (did DeviceID) String() string {
	return string(did)
}

func (gn GroupName) String() string {
	return string(gn)
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

func (d DeviceAttributes) MarshalJSON() ([]byte, error) {
	attrsArray := make([]DeviceAttribute, 0, len(d))

	for a, v := range d {
		nv := v
		nv.Name = a
		attrsArray = append(attrsArray, nv)
	}

	return json.Marshal(attrsArray)
}

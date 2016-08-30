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
	"github.com/asaskevich/govalidator"
)

type GroupID string

// Group wrapper
type Group struct {
	//system-generated group ID
	ID GroupID `json:"id" bson:"_id,omitempty"`

	// Group name assigned by the user
	Name string `json:"name" bson:"name,omitempty" valid:"required"`

	// Optional group description provided by the user
	Description *string `json:"description,omitempty" bson:"description,omitempty" valid:"optional"`

	// List of device IDs
	DeviceIDs []DeviceID `json:"device_ids" bson:"device_ids,omitempty" valid:"required"`
}

func (gid GroupID) String() string {
	return string(gid)
}

// Validate checkes structure according to valid tags
func (g *Group) Validate() error {
	if _, err := govalidator.ValidateStruct(g); err != nil {
		return err
	}
	return nil
}

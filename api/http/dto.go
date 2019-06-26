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
package http

import (
	"sort"
	"time"

	"github.com/mendersoftware/inventory/model"
)

type DeviceDto struct {
	ID         string                             `json:"id"`
	Attributes map[string][]model.DeviceAttribute `json:"attributes"`
	UpdatedTs  time.Time                          `json:"updated_ts"`
}

func NewDeviceDto(d *model.Device) *DeviceDto {
	dto := &DeviceDto{
		ID:         string(d.ID),
		UpdatedTs:  d.UpdatedTs,
		Attributes: map[string][]model.DeviceAttribute{},
	}

	for _, s := range model.AllScopes {
		dto.Attributes[s] = []model.DeviceAttribute{}
	}

	for _, a := range d.Attributes {
		dto.Attributes[a.Scope] = append(dto.Attributes[a.Scope], a)
	}

	for _, attrs := range dto.Attributes {
		sort.Slice(attrs, func(i, j int) bool {
			return attrs[j].Name > attrs[i].Name
		})
	}

	return dto
}

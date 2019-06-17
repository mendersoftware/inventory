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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/inventory/model"
)

var now = time.Now()

func TestNewDeviceDto(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		in  *model.Device
		out *DeviceDto
	}{
		{
			in: &model.Device{
				ID:        model.DeviceID("123"),
				UpdatedTs: now,
			},
			out: &DeviceDto{
				ID: "123",
				Attributes: map[string][]model.DeviceAttribute{
					"system":    []model.DeviceAttribute{},
					"inventory": []model.DeviceAttribute{},
					"identity":  []model.DeviceAttribute{},
					"custom":    []model.DeviceAttribute{},
				},
				UpdatedTs: now,
			},
		},
		{
			in: &model.Device{
				ID: model.DeviceID("123"),
				Attributes: model.DeviceAttributes{
					"foo": model.DeviceAttribute{
						Name:  "foo",
						Value: "bar",
						Scope: "inventory",
					},
				},
				UpdatedTs: now,
			},
			out: &DeviceDto{
				ID: "123",
				Attributes: map[string][]model.DeviceAttribute{
					"inventory": []model.DeviceAttribute{
						{
							Name:  "foo",
							Value: "bar",
							Scope: "inventory",
						},
					},
					"system":   []model.DeviceAttribute{},
					"identity": []model.DeviceAttribute{},
					"custom":   []model.DeviceAttribute{},
				},
				UpdatedTs: now,
			},
		},
		{
			in: &model.Device{
				ID: model.DeviceID("123"),
				Attributes: model.DeviceAttributes{
					"foo": model.DeviceAttribute{
						Name:  "foo",
						Value: []interface{}{"foo", "bar"},
						Scope: "system",
					},
				},
				UpdatedTs: now,
			},
			out: &DeviceDto{
				ID: "123",
				Attributes: map[string][]model.DeviceAttribute{
					"system": []model.DeviceAttribute{
						{
							Name:  "foo",
							Value: []interface{}{"foo", "bar"},
							Scope: "system",
						},
					},
					"inventory": []model.DeviceAttribute{},
					"identity":  []model.DeviceAttribute{},
					"custom":    []model.DeviceAttribute{},
				},
				UpdatedTs: now,
			},
		},
		{
			in: &model.Device{
				ID: model.DeviceID("123"),
				Attributes: model.DeviceAttributes{
					"foo-id": model.DeviceAttribute{
						Name:  "foo",
						Value: []interface{}{"foo", "bar"},
						Scope: "identity",
					},
					"bar-id": model.DeviceAttribute{
						Name:  "bar",
						Value: []interface{}{1.2, 3.4},
						Scope: "identity",
					},
					"foo-sys": model.DeviceAttribute{
						Name:  "foo",
						Value: "val",
						Scope: "system",
					},
					"bar-sys": model.DeviceAttribute{
						Name:  "bar",
						Value: 123,
						Scope: "system",
					},
					"baz-inv": model.DeviceAttribute{
						Name:  "baz",
						Value: []interface{}{"baz"},
						Scope: "inventory",
					},
				},
				UpdatedTs: now,
			},
			out: &DeviceDto{
				ID: "123",
				Attributes: map[string][]model.DeviceAttribute{
					"identity": []model.DeviceAttribute{
						{
							Name:  "bar",
							Value: []interface{}{1.2, 3.4},
							Scope: "identity",
						},
						{
							Name:  "foo",
							Value: []interface{}{"foo", "bar"},
							Scope: "identity",
						},
					},
					"inventory": []model.DeviceAttribute{
						{
							Name:  "baz",
							Value: []interface{}{"baz"},
							Scope: "inventory",
						},
					},
					"system": []model.DeviceAttribute{
						model.DeviceAttribute{
							Name:  "bar",
							Value: 123,
							Scope: "system",
						},
						model.DeviceAttribute{
							Name:  "foo",
							Value: "val",
							Scope: "system",
						},
					},
					"custom": []model.DeviceAttribute{},
				},
				UpdatedTs: now,
			},
		},
	}

	for i := range tcs {
		tc := tcs[i]
		t.Run(fmt.Sprintf("tc %d", i), func(t *testing.T) {

			dto := NewDeviceDto(tc.in)
			assert.Equal(t, tc.out, dto)
		})
	}
}

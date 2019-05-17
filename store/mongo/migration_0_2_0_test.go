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
package mongo

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/stretchr/testify/assert"

	"github.com/mendersoftware/inventory/model"
)

func TestMigration_0_2_0(t *testing.T) {
	cases := map[string]struct {
		inDevs  []interface{}
		outDevs []model.Device
		tenant  string
	}{
		"single dev": {
			inDevs: []interface{}{
				model.Device{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {Name: "foo", Value: "val3", Description: strPtr("desc")},
						"bar": {Name: "bar", Value: 3.0, Description: strPtr("desc")},
					},
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"inventory-foo": {Name: "foo", Value: "val3", Description: strPtr("desc"), Scope: "inventory"},
						"inventory-bar": {Name: "bar", Value: 3.0, Description: strPtr("desc"), Scope: "inventory"},
					},
				},
			},
		},
		"a couple devs, same attributes": {
			inDevs: []interface{}{
				model.Device{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {Name: "foo", Value: "val3", Description: strPtr("desc")},
						"bar": {Name: "bar", Value: 3.0, Description: strPtr("desc")},
					},
				},
				model.Device{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {Name: "foo2", Value: "val32", Description: strPtr("desc2")},
						"bar": {Name: "bar2", Value: 2.0, Description: strPtr("desc2")},
					},
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"inventory-foo": {Name: "foo", Value: "val3", Description: strPtr("desc"), Scope: "inventory"},
						"inventory-bar": {Name: "bar", Value: 3.0, Description: strPtr("desc"), Scope: "inventory"},
					},
				},
				{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"inventory-foo": {Name: "foo2", Value: "val32", Description: strPtr("desc2"), Scope: "inventory"},
						"inventory-bar": {Name: "bar2", Value: 2.0, Description: strPtr("desc2"), Scope: "inventory"},
					},
				},
			},
		},
		"a couple devs, diff attributes": {
			inDevs: []interface{}{
				model.Device{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {Name: "foo", Value: "val3", Description: strPtr("desc")},
						"bar": {Name: "bar", Value: 3.0, Description: strPtr("desc")},
					},
				},
				model.Device{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"baz": {Name: "baz", Value: "val", Description: strPtr("desc")},
					},
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"inventory-foo": {Name: "foo", Value: "val3", Description: strPtr("desc"), Scope: "inventory"},
						"inventory-bar": {Name: "bar", Value: 3.0, Description: strPtr("desc"), Scope: "inventory"},
					},
				},
				{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"inventory-baz": {Name: "baz", Value: "val", Description: strPtr("desc"), Scope: "inventory"},
					},
				},
			},
		},
	}

	for n, tc := range cases {
		t.Run(fmt.Sprintf("tc %s", n), func(t *testing.T) {
			ctx := context.Background()

			if tc.tenant != "" {
				ctx = identity.WithContext(ctx, &identity.Identity{
					Tenant: tc.tenant,
				})
			}

			//setup
			db.Wipe()
			s := db.Session()
			ds := NewDataStoreMongoWithSession(s)
			defer ds.session.Close()

			prep_0_1_0(t, ctx, ds)

			mig_0_2_0 := migration_0_2_0{
				ms:  ds,
				ctx: ctx,
			}

			c := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)
			err := c.Insert(tc.inDevs...)
			assert.NoError(t, err)

			//test
			err = mig_0_2_0.Up(migrate.MakeVersion(0, 1, 0))

			assert.NoError(t, err)

			var dbdevs []*model.Device
			devsColl := s.DB(mstore.DbFromContext(ctx, DbName)).C(DbDevicesColl)
			err = devsColl.Find(nil).All(&dbdevs)

			assert.NoError(t, err)
			assert.Len(t, dbdevs, len(tc.outDevs))

			sort.Slice(dbdevs, func(i, j int) bool { return dbdevs[i].ID < dbdevs[j].ID })

			for i := range dbdevs {
				compareDevsWithoutTimestamps(t, &tc.outDevs[i], dbdevs[i])
			}
		})
	}
}

func prep_0_1_0(t *testing.T, ctx context.Context, ds *DataStoreMongo) {
	m := migrate.DummyMigrator{
		Session:     ds.session,
		Db:          mstore.DbFromContext(ctx, DbName),
		Automigrate: true,
	}

	last := migrate.MakeVersion(0, 1, 0)
	err := m.Apply(ctx, last, nil)
	assert.NoError(t, err)
}

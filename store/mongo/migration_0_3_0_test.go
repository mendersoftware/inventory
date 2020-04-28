// Copyright 2020 Northern.tech AS
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
	"time"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendersoftware/inventory/model"
)

func TestMigration_0_3_0(t *testing.T) {
	testTimestamp := time.Now()
	cases := map[string]struct {
		inDevs  []interface{}
		outDevs []model.Device
		tenant  string
	}{
		"single dev": {
			inDevs: []interface{}{
				legacyDevice{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {
							Name:        "foo",
							Value:       "val3",
							Description: strPtr("desc"),
						},
					},
					Group:     "foobar",
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: model.DeviceAttributes{{
						Name:        "foo",
						Value:       "val3",
						Description: strPtr("desc"),
						Scope:       model.AttrScopeInventory,
					}, {
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "group",
						Value: "foobar",
						Scope: model.AttrScopeIdentity,
					}},
					Group: "foobar",
				},
			},
		},
		"one ungrouped and one grouped device": {
			inDevs: []interface{}{
				legacyDevice{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {
							Name:        "foo",
							Value:       "val3",
							Description: strPtr("desc"),
						},
					},
					Group:     "foobar",
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
				legacyDevice{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {
							Name:        "foo",
							Value:       "val3",
							Description: strPtr("desc"),
						},
					},
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: model.DeviceAttributes{{
						Name:        "foo",
						Value:       "val3",
						Description: strPtr("desc"),
						Scope:       model.AttrScopeInventory,
					}, {
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "group",
						Value: "foobar",
						Scope: model.AttrScopeIdentity,
					}},
					Group: "foobar",
				}, {
					ID: model.DeviceID("2"),
					Attributes: model.DeviceAttributes{{
						Name:        "foo",
						Value:       "val3",
						Description: strPtr("desc"),
						Scope:       model.AttrScopeInventory,
					}, {
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}},
				},
			},
		},
		"multiple devs, with tenant": {
			tenant: "foobarbaz",
			inDevs: []interface{}{
				legacyDevice{
					ID: model.DeviceID("1"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {
							Name:        "foo",
							Value:       "val3",
							Description: strPtr("desc"),
						},
					},
					Group:     "foobar",
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
				legacyDevice{
					ID: model.DeviceID("2"),
					Attributes: map[string]model.DeviceAttribute{
						"foo": {
							Name:        "foo",
							Value:       "val3",
							Description: strPtr("desc"),
						},
					},
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
				legacyDevice{
					ID:        model.DeviceID("3"),
					UpdatedTs: testTimestamp,
					CreatedTs: testTimestamp,
				},
			},
			outDevs: []model.Device{
				{
					ID: model.DeviceID("1"),
					Attributes: model.DeviceAttributes{{
						Name:        "foo",
						Value:       "val3",
						Description: strPtr("desc"),
						Scope:       model.AttrScopeInventory,
					}, {
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "group",
						Value: "foobar",
						Scope: model.AttrScopeIdentity,
					}},
					Group: "foobar",
				}, {
					ID: model.DeviceID("2"),
					Attributes: model.DeviceAttributes{{
						Name:        "foo",
						Value:       "val3",
						Description: strPtr("desc"),
						Scope:       model.AttrScopeInventory,
					}, {
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}},
				}, {
					ID: model.DeviceID("3"),
					Attributes: model.DeviceAttributes{{
						Name:  "created_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}, {
						Name:  "updated_ts",
						Value: testTimestamp,
						Scope: model.AttrScopeIdentity,
					}},
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

			// setup
			db.Wipe()
			s := db.Client()
			ds := NewDataStoreMongoWithSession(s).(*DataStoreMongo)

			migrations := []migrate.Migration{
				&migration_0_2_0{
					ms:  ds,
					ctx: ctx,
				},
				&migration_0_3_0{
					ms:  ds,
					ctx: ctx,
				},
			}
			migrator := &migrate.SimpleMigrator{
				Client:      s,
				Db:          mstore.DbFromContext(ctx, DbName),
				Automigrate: true,
			}

			c := s.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
			_, err := c.InsertMany(ctx, tc.inDevs)
			assert.NoError(t, err)

			err = migrator.Apply(ctx, migrate.MakeVersion(0, 3, 0), migrations)
			assert.NoError(t, err)

			var dbdevs []*model.Device
			devsColl := s.Database(mstore.DbFromContext(ctx, DbName)).Collection(DbDevicesColl)
			cursor, err := devsColl.Find(ctx, bson.M{})
			cursor.All(ctx, &dbdevs)

			assert.NoError(t, err)
			assert.Len(t, dbdevs, len(tc.outDevs))

			sort.Slice(dbdevs, func(i, j int) bool { return dbdevs[i].ID < dbdevs[j].ID })
			for i := range dbdevs {
				compareDevsWithoutTimestamps(t, &tc.outDevs[i], dbdevs[i])
			}
		})
	}
}

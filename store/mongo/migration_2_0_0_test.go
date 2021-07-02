// Copyright 2021 Northern.tech AS
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
	"testing"
	"time"

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore_v1 "github.com/mendersoftware/go-lib-micro/store"
	mstore "github.com/mendersoftware/go-lib-micro/store/v2"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
)

func TestMigration_2_0_0(t *testing.T) {
	now := time.Now().UTC().Round(time.Second).Truncate(0)

	cases := map[string]struct {
		devicesPerTenant map[string][]model.Device
	}{
		"single tenant": {
			devicesPerTenant: map[string][]model.Device{
				"": {
					{
						ID: "1",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
					{
						ID: "2",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
				},
			},
		},
		"multi-tenant": {
			devicesPerTenant: map[string][]model.Device{
				"tenant1": {
					{
						ID: "1",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
					{
						ID: "2",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
				},
				"tenant2": {
					{
						ID: "3",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
					{
						ID: "4",
						Attributes: model.DeviceAttributes{
							model.DeviceAttribute{
								Name:  "attribute",
								Value: "value",
								Scope: model.AttrScopeInventory,
							},
						},
						CreatedTs: now,
						UpdatedTs: now,
					},
				},
			},
		},
	}

	for n, tc := range cases {
		t.Run(n, func(t *testing.T) {
			ctx := context.Background()
			db.Wipe()

			s := db.Client()

			// create the documents in the tenant-specific databases
			numDevices := int64(0)
			for tenant, devices := range tc.devicesPerTenant {
				ctx := ctx
				if tenant != "" {
					ctx = identity.WithContext(ctx, &identity.Identity{
						Tenant: tenant,
					})
				}

				// run the migrations up to 1.0.2
				ds := NewDataStoreMongoWithSession(s).(*DataStoreMongo)
				migrations := []migrate.Migration{
					&migration_0_2_0{
						ms:  ds,
						ctx: ctx,
					},
					&migration_1_0_0{
						ms:  ds,
						ctx: ctx,
					},
					&migration_1_0_1{
						ms:  ds,
						ctx: ctx,
					},
					&migration_1_0_2{
						ms:  ds,
						ctx: ctx,
					},
				}
				migrator := &migrate.SimpleMigrator{
					Client:      s,
					Db:          mstore.DbFromContext(ctx, DbName),
					Automigrate: true,
				}

				err := migrator.Apply(ctx, migrate.MakeVersion(1, 0, 2), migrations)
				assert.NoError(t, err)

				// insert the documents
				dbName := mstore_v1.DbNameForTenant(tenant, DbName)
				c := s.Database(dbName).Collection(DbDevicesColl)
				docs := make([]interface{}, len(devices))
				for i, v := range devices {
					docs[i] = v
				}
				_, err = c.InsertMany(ctx, docs)
				assert.NoError(t, err)

				numDevices += int64(len(docs))
			}

			// run the 2.0.0 migration for the non-tenant-specific database
			ds := NewDataStoreMongoWithSession(s).(*DataStoreMongo)
			migrations := []migrate.Migration{
				&migration_2_0_0{
					ms:  ds,
					ctx: ctx,
				},
			}
			migrator := &migrate.SimpleMigrator{
				Client:      s,
				Db:          mstore.DbFromContext(ctx, DbName),
				Automigrate: true,
			}

			err := migrator.Apply(ctx, migrate.MakeVersion(2, 0, 0), migrations)
			assert.NoError(t, err)

			// verify the total number of devices
			count, err := s.Database(DbName).Collection(DbDevicesColl).CountDocuments(ctx, bson.D{})
			assert.Equal(t, numDevices, count)

			// verify the devices per tenant
			for tenant, devices := range tc.devicesPerTenant {
				ctx := ctx
				if tenant != "" {
					ctx = identity.WithContext(ctx, &identity.Identity{
						Tenant: tenant,
					})
				}

				ds := NewDataStoreMongoWithSession(s).(*DataStoreMongo)
				foundDevices, count, err := ds.GetDevices(ctx, store.ListQuery{})
				assert.NoError(t, err)
				assert.Equal(t, len(devices), count)

				for i, _ := range foundDevices {
					foundDevices[i].CreatedTs = foundDevices[i].CreatedTs.Truncate(0)
					foundDevices[i].UpdatedTs = foundDevices[i].UpdatedTs.Truncate(0)
				}
				assert.Equal(t, devices, foundDevices)
			}
		})
	}
}

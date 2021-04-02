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

	"github.com/mendersoftware/go-lib-micro/identity"
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstorev1 "github.com/mendersoftware/go-lib-micro/store"
	mstore "github.com/mendersoftware/go-lib-micro/store/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mopts "go.mongodb.org/mongo-driver/mongo/options"
)

const findBatchSize = 100

type migration_2_0_0 struct {
	ms  *DataStoreMongo
	ctx context.Context
}

func (m *migration_2_0_0) Up(from migrate.Version) error {
	id := identity.FromContext(m.ctx)
	if id != nil && id.Tenant != "" {
		return nil
	}

	ctx := context.Background()
	client := m.ms.client
	dbs, err := migrate.GetTenantDbs(ctx, client, mstorev1.IsTenantDb(DbName))
	if err != nil {
		return err
	}
	dbs = append([]string{DbName}, dbs...)

	collections := map[string]struct {
		Indexes []mongo.IndexModel
	}{
		DbDevicesColl: {
			Indexes: []mongo.IndexModel{
				{
					Keys: bson.D{
						{Key: mstore.FieldTenantID, Value: 1},
						{Key: DbDevId, Value: 1},
					},
					Options: mopts.Index().
						SetName(mstore.FieldTenantID + "_" + DbDevId),
				},
				{
					Keys: bson.D{
						{Key: mstore.FieldTenantID, Value: 1},
						{Key: DbDevUpdatedTs, Value: 1},
					},
					Options: mopts.Index().
						SetName(mstore.FieldTenantID + "_" + DbDevUpdatedTs),
				},
			},
		},
	}

	// for each collection...
	for collection, config := range collections {
		coll := client.Database(DbName).Collection(collection)
		// drop all the existing indexes, ignoring the errors
		_, _ = coll.Indexes().DropAll(ctx)

		// create the new indexes
		_, err := coll.Indexes().CreateMany(ctx, config.Indexes)
		if err != nil {
			return err
		}
	}

	// for each database...
	for _, db := range dbs {
		tenantID := mstorev1.TenantFromDbName(db, DbName)
		ctx := identity.WithContext(ctx, &identity.Identity{
			Tenant: tenantID,
		})
		// for each collection...
		for collection, _ := range collections {
			// get all the documents in the collection
			findOptions := mopts.Find().
				SetBatchSize(findBatchSize).
				SetSort(bson.D{{Key: DbDevId, Value: 1}})
			coll := client.Database(db).Collection(collection)
			collOut := client.Database(DbName).Collection(collection)
			cur, err := coll.Find(ctx, bson.D{}, findOptions)
			if err != nil {
				return err
			}
			defer cur.Close(ctx)

			writes := make([]mongo.WriteModel, 0, findBatchSize)

			// migrate the documents
			for cur.Next(ctx) {
				item := bson.D{}
				err := cur.Decode(&item)
				if err != nil {
					return err
				}

				item = mstore.WithTenantID(ctx, item)
				if db == DbName {
					filter := bson.D{}
					for _, i := range item {
						if i.Key == DbDevId {
							filter = append(filter, i)
						}
					}
					writes = append(writes, mongo.NewReplaceOneModel().SetFilter(filter).SetReplacement(item))
				} else {
					writes = append(writes, mongo.NewInsertOneModel().SetDocument(item))
				}
				if len(writes) == findBatchSize {
					_, err := collOut.BulkWrite(ctx, writes)
					if err != nil {
						return err
					}
					writes = writes[:0]
				}
			}
			if len(writes) > 0 {
				_, err := collOut.BulkWrite(ctx, writes)
				if err != nil {
					return err
				}
			}
		}

	}

	return nil
}

func (m *migration_2_0_0) Version() migrate.Version {
	return migrate.MakeVersion(2, 0, 0)
}

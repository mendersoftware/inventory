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

	"go.mongodb.org/mongo-driver/bson"
	mopts "go.mongodb.org/mongo-driver/mongo/options"

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/mendersoftware/inventory/model"
)

type migration_0_3_0 struct {
	ms  *DataStoreMongo
	ctx context.Context
}

// Up creates timestamp and group attributes under the identity scope.
// The values reflect the values previously held in the root of the document.
func (m *migration_0_3_0) Up(from migrate.Version) error {
	const identityScope = DbDevAttributes + "." + model.AttrScopeIdentity

	databaseName := mstore.DbFromContext(m.ctx, DbName)
	collDevs := m.ms.client.Database(databaseName).Collection(DbDevicesColl)
	upsertOpts := mopts.Update().SetUpsert(true)

	// Add timestamp attributes to the identity scope
	cursor, err := collDevs.Find(m.ctx, bson.M{})
	if err != nil {
		return err
	}
	defer cursor.Close(m.ctx)
	for cursor.Next(m.ctx) {
		var dev model.Device
		err := cursor.Decode(&dev)
		if err != nil {
			return err
		}
		dev.Attributes = append(dev.Attributes,
			model.DeviceAttribute{
				Name:  "created_ts",
				Value: dev.CreatedTs,
				Scope: model.AttrScopeIdentity,
			},
			model.DeviceAttribute{
				Name:  "updated_ts",
				Value: dev.UpdatedTs,
				Scope: model.AttrScopeIdentity,
			},
		)
		if dev.Group != "" {
			dev.Attributes = append(dev.Attributes,
				model.DeviceAttribute{
					Name:  DbDevGroup,
					Value: dev.Group,
					Scope: model.AttrScopeIdentity,
				},
			)
		}
		upsert, err := makeAttrUpsert(dev.Attributes)
		if err != nil {
			return err
		}

		_, err = collDevs.UpdateOne(
			m.ctx,
			bson.M{"_id": dev.ID},
			bson.M{"$set": upsert},
			upsertOpts,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *migration_0_3_0) Version() migrate.Version {
	return migrate.MakeVersion(0, 3, 0)
}

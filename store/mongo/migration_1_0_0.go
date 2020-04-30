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

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/mendersoftware/inventory/model"
)

type migration_1_0_0 struct {
	ms  *DataStoreMongo
	ctx context.Context
}

// Up creates timestamp and group attributes under the identity scope.
// The values reflect the values previously held in the root of the document.
func (m *migration_1_0_0) Up(from migrate.Version) error {
	const systemScope = DbDevAttributes + "." + model.AttrScopeSystem

	databaseName := mstore.DbFromContext(m.ctx, DbName)
	collDevs := m.ms.client.Database(databaseName).Collection(DbDevicesColl)

	// Move timestamps to identity scope.
	_, err := collDevs.UpdateMany(m.ctx, bson.M{}, bson.M{
		"$rename": bson.M{
			"created_ts": systemScope + "-created_ts.value",
			"updated_ts": systemScope + "-updated_ts.value",
		},
		"$set": bson.M{
			systemScope +
				"-created_ts.name": "created_ts",
			systemScope +
				"-created_ts.scope": model.AttrScopeSystem,
			systemScope +
				"-updated_ts.name": "updated_ts",
			systemScope +
				"-updated_ts.scope": model.AttrScopeSystem,
		},
	})
	if err != nil {
		return nil
	}

	// For devices in a group: move to attribute.
	_, err = collDevs.UpdateMany(
		m.ctx,
		bson.M{DbDevGroup: bson.M{"$exists": true}},
		bson.M{
			"$rename": bson.M{DbDevGroup: DbDevAttributesGroupValue},
			"$set": bson.M{
				DbDevAttributesGroup + "." +
					DbDevAttributesName: DbDevGroup,
				DbDevAttributesGroup + "." +
					DbDevAttributesScope: model.AttrScopeIdentity,
			},
		})
	return err
}

func (m *migration_1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}

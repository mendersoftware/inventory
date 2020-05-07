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
	createdNameField := makeAttrField(
		model.AttrNameCreated,
		model.AttrScopeSystem,
		DbDevAttributesName,
	)
	createdScopeField := makeAttrField(
		model.AttrNameCreated,
		model.AttrScopeSystem,
		DbDevAttributesScope,
	)
	createdValueField := makeAttrField(
		model.AttrNameCreated,
		model.AttrScopeSystem,
		DbDevAttributesValue,
	)
	updatedNameField := makeAttrField(
		model.AttrNameUpdated,
		model.AttrScopeSystem,
		DbDevAttributesName,
	)
	updatedScopeField := makeAttrField(
		model.AttrNameUpdated,
		model.AttrScopeSystem,
		DbDevAttributesScope,
	)
	updatedValueField := makeAttrField(
		model.AttrNameUpdated,
		model.AttrScopeSystem,
		DbDevAttributesValue,
	)
	_, err := collDevs.UpdateMany(m.ctx, bson.M{}, bson.M{
		"$rename": bson.M{
			model.AttrNameCreated: createdValueField,
			model.AttrNameUpdated: updatedValueField,
		},
		"$set": bson.M{
			createdNameField:  model.AttrNameCreated,
			createdScopeField: model.AttrScopeSystem,
			updatedNameField:  model.AttrNameUpdated,
			updatedScopeField: model.AttrScopeSystem,
		},
	})
	if err != nil {
		return nil
	}

	groupNameField := makeAttrField(
		model.AttrNameGroup,
		model.AttrScopeSystem,
		DbDevAttributesName,
	)
	groupScopeField := makeAttrField(
		model.AttrNameGroup,
		model.AttrScopeSystem,
		DbDevAttributesScope,
	)
	// For devices in a group: move to attribute.
	_, err = collDevs.UpdateMany(
		m.ctx,
		bson.M{DbDevGroup: bson.M{"$exists": true}},
		bson.M{
			"$rename": bson.M{
				model.AttrNameGroup: DbDevAttributesGroupValue,
			},
			"$set": bson.M{
				groupNameField:  model.AttrNameGroup,
				groupScopeField: model.AttrScopeSystem,
			},
		})
	return err
}

func (m *migration_1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}

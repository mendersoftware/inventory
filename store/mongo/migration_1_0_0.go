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
	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
	mstore "github.com/mendersoftware/go-lib-micro/store"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type migration_1_0_0 struct {
	ms  *DataStoreMongo
	ctx context.Context
}

func (m *migration_1_0_0) Up(from migrate.Version) error {
	// rename every group to attribute, add name and scope fields
	databaseName := mstore.DbFromContext(m.ctx, DbName)
	coll := m.ms.client.Database(databaseName).Collection(DbDevicesColl)
	_, err := coll.UpdateMany(m.ctx, bson.M{}, bson.M{"$rename": bson.M{"group": "attributes.identity-group.value"}, "$set": bson.M{"attributes.identity-group.scope": "identity", "attributes.identity-group.name": "group"}})
	if err != nil {
		return errors.Wrapf(err, "failed to move group to attribute")
	}

	return nil
}

func (m *migration_1_0_0) Version() migrate.Version {
	return migrate.MakeVersion(1, 0, 0)
}

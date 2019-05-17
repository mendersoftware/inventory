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

	"github.com/pkg/errors"

	"github.com/mendersoftware/go-lib-micro/mongo/migrate"
)

type migration_0_2_0 struct {
	ms  *DataStoreMongo
	ctx context.Context
}

func (m *migration_0_2_0) Up(from migrate.Version) error {
	s := m.ms.session.Copy()
	defer s.Close()

	return nil
}

func (m *migration_0_2_0) Version() migrate.Version {
	return migrate.MakeVersion(0, 2, 0)
}

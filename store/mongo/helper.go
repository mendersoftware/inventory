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

	"github.com/mendersoftware/inventory/model"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func DeviceFindById(ctx context.Context, c *mongo.Collection, id model.DeviceID, dst *model.Device) error {
	cur, err := c.Find(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}

	cur.Next(ctx)

	// create a value into which the single document can be decoded
	err = cur.Decode(dst)
	if err != nil {
		return err
	}

	return nil
}

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
package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeviceAttributesUnmarshal(t *testing.T) {

	var da DeviceAttributes
	j := `[
{"name": "foo", "value": "bar"},
{"name": "baz", "value": "zen"}
]`
	err := json.Unmarshal([]byte(j), &da)
	assert.NoError(t, err)

	exp := DeviceAttributes{
		"foo": {
			Name:  "foo",
			Value: "bar",
		},
		"baz": {
			Name:  "baz",
			Value: "zen",
		},
	}

	assert.ObjectsAreEqualValues(exp, da)
}

func TestDeviceAttributesMarshal(t *testing.T) {

	da := DeviceAttributes{
		"foo": {
			Value: "bar",
		},
		"bar": {
			Name:  "bar",
			Value: []int{1, 2, 3},
		},
	}

	data, err := json.Marshal(&da)
	assert.NoError(t, err)

	// Map keys are retrieved in random order, hence the order of elements in the
	// list may differ with each run, thus preventing us from using
	// assert.JSONEq(). Since we're interested in how the output JSON is
	// formatted, use 2 possible variants and check that the output matches at
	// least one.
	exp1 := `[{"name":"foo","value":"bar","scope":"inventory"},{"name":"bar","value":[1,2,3],"scope":"inventory"}]`
	exp2 := `[{"name":"bar","value":[1,2,3],"scope":"inventory"},{"name":"foo","value":"bar","scope":"inventory"}]`
	if string(data) != exp1 && string(data) != exp2 {
		assert.Fail(t, "unexpected JSON marshal output, got:", string(data))
	}

	var uda DeviceAttributes
	json.Unmarshal(data, &da)

	assert.ObjectsAreEqualValues(uda, da)

	var daEmpty DeviceAttributes
	data, err = json.Marshal(&daEmpty)
	assert.NoError(t, err)
	assert.Equal(t, "[]", string(data))
}

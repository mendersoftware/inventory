// Copyright 2017 Northern.tech AS
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
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/ant0ine/go-json-rest/rest/test"
	"github.com/mendersoftware/go-lib-micro/requestid"
	"github.com/mendersoftware/go-lib-micro/requestlog"
	mt "github.com/mendersoftware/go-lib-micro/testing"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	inventory "github.com/mendersoftware/inventory/inv"
	minventory "github.com/mendersoftware/inventory/inv/mocks"
	"github.com/mendersoftware/inventory/model"
	"github.com/mendersoftware/inventory/store"
	"github.com/mendersoftware/inventory/utils"
)

func contextMatcher() interface{} {
	return mock.MatchedBy(func(c context.Context) bool {
		return true
	})
}

func ToJson(data interface{}) string {
	j, _ := json.Marshal(data)
	return string(j)
}

// test.HasHeader only tests the first header,
// so create a wrapper for headers with multiple values
func HasHeader(hdr, val string, r *test.Recorded) bool {
	rec := r.Recorder
	for _, v := range rec.Header()[hdr] {
		if v == val {
			return true
		}
	}

	return false
}

func ExtractHeader(hdr, val string, r *test.Recorded) string {
	rec := r.Recorder
	for _, v := range rec.Header()[hdr] {
		if v == val {
			return v
		}
	}

	return ""
}

func RestError(status string) map[string]interface{} {
	return map[string]interface{}{"error": status, "request_id": "test"}
}

func runTestRequest(t *testing.T, handler http.Handler, req *http.Request, resp utils.JSONResponseParams) {
	req.Header.Add(requestid.RequestIdHeader, "test")
	recorded := test.RunRequest(t, handler, req)
	utils.CheckRecordedResponse(t, recorded, resp)
}

func makeMockApiHandler(t *testing.T, i inventory.InventoryApp) http.Handler {
	handlers := NewInventoryApiHandlers(i)
	assert.NotNil(t, handlers)

	app, err := handlers.GetApp()
	assert.NotNil(t, app)
	assert.NoError(t, err)

	api := rest.NewApi()
	api.Use(
		&requestlog.RequestLogMiddleware{},
		&requestid.RequestIdMiddleware{},
	)
	api.SetApp(app)

	return api.MakeHandler()
}

func mockListDevices(num int) []model.Device {
	var devs []model.Device
	for i := 0; i < num; i++ {
		devs = append(devs, model.Device{ID: model.DeviceID(strconv.Itoa(i))})
	}
	return devs
}

func mockListDeviceIDs(num int) []model.DeviceID {
	var devs []model.DeviceID
	for i := 0; i < num; i++ {
		devs = append(devs, model.DeviceID(strconv.Itoa(i)))
	}
	return devs
}

func TestApiInventoryGetDevices(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		listDevicesNum int
		listDevicesErr error
		inReq          *http.Request
		resp           utils.JSONResponseParams
	}{
		"get all devices in group": {
			listDevicesNum: 3,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5&group=foo", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(3),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "group=foo&page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "group=foo&page=1&per_page=5", "first"),
					},
				},
			},
		},
		"valid pagination, no next page": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
				},
			},
		},
		"valid pagination, with next page": {
			listDevicesNum: 9,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
				},
			},
		},
		"invalid pagination - page format": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=foo&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - per_page format": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=foo", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("per_page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - bounds": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=0&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmLimit("page")),
				OutputHeaders:    nil,
			},
		},
		"valid attribute filter": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=qe:123:123:123", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "attr_name1=qe%3A123%3A123%3A123&page=1&per_page=5", "first"),
					},
				},
			},
		},
		"invalid attribute filter operator": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=neq:123", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("invalid filter operator"),
				OutputHeaders:    nil,
			},
		},
		"valid sort order value": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?sort=attr_name1:asc&page=1&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5&sort=attr_name1%3Aasc", "first"),
					},
				},
			},
		},
		"invalid sort order value": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&sort=attr_name1:gte", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("invalid sort order"),
				OutputHeaders:    nil,
			},
		},
		"valid has_group": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?has_group=true&page=1&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "has_group=true&page=1&per_page=5", "first"),
					},
				},
			},
		},
		"invalid has_group": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&has_group=asd", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("has_group")),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevices error": {
			listDevicesNum: 5,
			listDevicesErr: errors.New("inventory error"),
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     500,
				OutputBodyObject: RestError("internal error"),
				OutputHeaders:    nil,
			},
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("ListDevices",
			ctx,
			mock.AnythingOfType("store.ListQuery"),
		).Return(mockListDevices(testCase.listDevicesNum), testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
	}
}

func TestApiInventoryAddDevice(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		utils.JSONResponseParams

		inReq *http.Request

		inventoryErr error
	}{
		"empty body": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				nil),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
		},
		"garbled body": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				"foo bar"),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type model.Device"),
			},
		},
		"body formatted ok, all fields present": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"name": "a1", "value": "00:00:00:01", "description": "ddd"},
						{"name": "a2", "value": 123.2, "description": "ddd"},
						{"name": "a3", "value": []interface{}{"00:00:00:01", "00"}, "description": "ddd"},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusCreated,
				OutputBodyObject: nil,
				OutputHeaders:    map[string][]string{"Location": {"devices/id-0001"}},
			},
		},
		"body formatted ok, wrong attributes type": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{
					"id":         "id-0001",
					"attributes": 123,
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal number into Go struct field Device.attributes of type []model.DeviceAttribute"),
			},
		},
		"body formatted ok, 'id' missing": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("ID: non zero value required;"),
			},
		},
		"body formatted ok, incorrect attribute value": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"name": "asd", "value": []interface{}{"asd", 123}},
						{"name": "asd2", "value": []interface{}{123, "asd"}},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Value: [asd 123] does not validate as deviceAttributeValueValidator;;;"),
			},
		},
		"body formatted ok, attribute name missing": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"value": "23"},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Name: non zero value required;;"),
			},
		},
		"body formatted ok, inv error": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{
							"name":  "name1",
							"value": "value4",
						},
					},
				},
			),
			inventoryErr: errors.New("internal error"),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("AddDevice",
			ctx,
			mock.AnythingOfType("*model.Device")).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiInventoryUpsertAttributes(t *testing.T) {
	t.Parallel()

	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		inReq  *http.Request
		inHdrs map[string]string

		inventoryErr error

		resp utils.JSONResponseParams
	}{
		"no auth": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusUnauthorized,
				OutputBodyObject: RestError("unauthorized"),
			},
		},

		"invalid auth": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inHdrs: map[string]string{
				"Authorization:": "foobar",
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusUnauthorized,
				OutputBodyObject: RestError("unauthorized"),
			},
		},

		"empty body": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
		},

		"garbled body": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				`{"foo": "bar"}`),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type []model.DeviceAttribute"),
			},
		},

		"body formatted ok, attribute name missing": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Value:       "value1",
						Description: strPtr("descr1"),
					},
					{
						Value:       2,
						Description: strPtr("descr2"),
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Name: non zero value required;"),
			},
		},

		"body formatted ok, attributes ok (all fields)": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Value:       "value1",
						Description: strPtr("descr1"),
					},
					{
						Name:        "name2",
						Value:       2,
						Description: strPtr("descr2"),
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok (all fields, arrays)": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Value:       []interface{}{"foo", "bar"},
						Description: strPtr("descr1"),
					},
					{
						Name:        "name2",
						Value:       []interface{}{1, 2, 3},
						Description: strPtr("descr2"),
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok (values only)": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:  "name1",
						Value: "value1",
					},
					{
						Name:  "name2",
						Value: 2,
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok (all fields), inventory err": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Value:       "value1",
						Description: strPtr("descr1"),
					},
					{
						Name:        "name2",
						Value:       2,
						Description: strPtr("descr2"),
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: errors.New("internal error"),
			resp: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("UpsertAttributes",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.DeviceAttributes")).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		rest.ErrorFieldName = "error"

		for k, v := range tc.inHdrs {
			tc.inReq.Header.Set(k, v)
		}

		runTestRequest(t, apih, tc.inReq, tc.resp)
	}
}

func makeDeviceAuthHeader(claim string) string {
	return fmt.Sprintf("Bearer foo.%s.bar",
		base64.StdEncoding.EncodeToString([]byte(claim)))
}

func strPtr(s string) *string {
	return &s
}

func TestApiInventoryDeleteDeviceGroup(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq *http.Request

		inventoryErr error
	}{
		"ok": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNoContent,
				OutputBodyObject: nil,
			},
		},
		"device group not found (or device's group is other than requested)": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
			inventoryErr: store.ErrDevNotFound,
		},
		"internal error": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("internal error"),
		},
	}

	for name, tc := range tcases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("UnsetDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.GroupName")).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiInventoryAddDeviceToGroup(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq *http.Request

		inventoryErr error
	}{
		"ok": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"_a-b-c_"}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNoContent,
				OutputBodyObject: nil,
			},
		},
		"device not found": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"abc"}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
			inventoryErr: store.ErrDevNotFound,
		},
		"empty group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group: non zero value required;"),
			},
			inventoryErr: nil,
		},
		"unsupported characters in group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"__+X@#$  ;"}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group name can only contain: upper/lowercase alphanum, -(dash), _(underscore)"),
			},
			inventoryErr: nil,
		},
		"non-ASCII characters in group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"ęą"}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group name can only contain: upper/lowercase alphanum, -(dash), _(underscore)"),
			},
			inventoryErr: nil,
		},
		"empty body": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode device group data: JSON payload is empty"),
			},
			inventoryErr: nil,
		},
		"internal error": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"abc"}),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("internal error"),
		},
	}

	for name, tc := range tcases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("UpdateDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.AnythingOfType("model.GroupName")).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiListGroups(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq        *http.Request
		outputGroups []model.GroupName

		inventoryErr error
	}{
		"some groups": {
			inReq:        test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups", nil),
			outputGroups: []model.GroupName{"foo", "bar"},
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: []string{"foo", "bar"},
			},
		},
		"no groups": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: []string{},
			},
		},
		"error": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("internal error"),
		},
	}

	for name, tc := range tcases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("ListGroups", ctx).Return(tc.outputGroups, tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiGetDevice(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq        *http.Request
		inDevId      model.DeviceID
		outputDevice *model.Device
		inventoryErr error
	}{
		"no device": {
			inDevId:      model.DeviceID("1"),
			inReq:        test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1", nil),
			outputDevice: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
		},
		"some device": {
			inDevId: model.DeviceID("2"),
			inReq:   test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/2", nil),
			outputDevice: &model.Device{
				ID:    model.DeviceID("2"),
				Group: model.GroupName("foo"),
			},
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus: http.StatusOK,
				OutputBodyObject: model.Device{
					ID:    model.DeviceID("2"),
					Group: model.GroupName("foo"),
				},
			},
		},
		"error": {
			inDevId: model.DeviceID("3"),
			inReq:   test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/3", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("internal error"),
		},
	}

	for name, tc := range tcases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("GetDevice", ctx, tc.inDevId).Return(tc.outputDevice, tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiInventoryGetDevicesByGroup(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		listDevicesNum int
		listDevicesErr error
		inReq          *http.Request
		resp           utils.JSONResponseParams
	}{
		"valid pagination, no next page": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDeviceIDs(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
				},
			},
		},
		"valid pagination, with next page": {
			listDevicesNum: 9,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDeviceIDs(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
				},
			},
		},
		"invalid pagination - page format": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=foo&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - per_page format": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=1&per_page=foo", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("per_page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - bounds": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=0&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmLimit("page")),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevicesByGroup error - group not found": {
			listDevicesNum: 5,
			listDevicesErr: store.ErrGroupNotFound,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     404,
				OutputBodyObject: RestError("group not found"),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevicesByGroup error - internal": {
			listDevicesNum: 5,
			listDevicesErr: errors.New("inventory error"),
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     500,
				OutputBodyObject: RestError("internal error"),
				OutputHeaders:    nil,
			},
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("ListDevicesByGroup",
			ctx,
			mock.AnythingOfType("model.GroupName"),
			mock.AnythingOfType("int"),
			mock.AnythingOfType("int"),
		).Return(mockListDeviceIDs(testCase.listDevicesNum), testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
	}
}

func TestApiGetDeviceGroup(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq *http.Request

		inventoryGroup model.GroupName
		inventoryErr   error
	}{

		/*
		   device w group
		   device n group
		   no device
		   generic error
		*/

		"device with group": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName("dev"),
			inventoryErr:   nil,

			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: map[string]interface{}{"group": "dev"},
			},
		},
		"device without group": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   nil,

			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: map[string]interface{}{"group": nil},
			},
		},
		"device not found": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   store.ErrDevNotFound,

			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
		},
		"generic inventory error": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   errors.New("inventory: internal error"),

			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
	}

	for name, tc := range tcases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("GetDeviceGroup",
			ctx,
			mock.AnythingOfType("model.DeviceID")).Return(tc.inventoryGroup, tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiDeleteDevice(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		utils.JSONResponseParams

		inReq        *http.Request
		inDevId      model.DeviceID
		inventoryErr error
	}{
		"no device": {
			inDevId:      model.DeviceID("1"),
			inReq:        test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/1", nil),
			inventoryErr: store.ErrDevNotFound,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"some device": {
			inDevId: model.DeviceID("2"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/2", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"error": {
			inDevId: model.DeviceID("3"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/3", nil),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("inventory internal error"),
		},
	}

	for name, tc := range tcases {
		t.Run(fmt.Sprintf("test case: %s", name), func(t *testing.T) {
			t.Parallel()

			inv := minventory.InventoryApp{}

			ctx := contextMatcher()

			inv.On("DeleteDevice",
				ctx,
				tc.inDevId).Return(tc.inventoryErr)

			apih := makeMockApiHandler(t, &inv)

			runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
		})
	}
}

func TestUserAdmApiCreateTenant(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		uaError error
		body    interface{}
		tenant  model.NewTenant

		checker mt.ResponseChecker
	}{
		"ok": {
			uaError: nil,
			body: map[string]interface{}{
				"tenant_id": "foobar",
			},
			tenant: model.NewTenant{ID: "foobar"},

			checker: mt.NewJSONResponse(
				http.StatusCreated,
				nil,
				nil,
			),
		},
		"error: useradm internal": {
			body: map[string]interface{}{
				"tenant_id": "failing-tenant",
			},
			uaError: errors.New("some internal error"),
			tenant:  model.NewTenant{ID: "failing-tenant"},

			checker: mt.NewJSONResponse(
				http.StatusInternalServerError,
				nil,
				restError("internal error"),
			),
		},
		"error: no tenant id": {
			body: map[string]interface{}{
				"tenant_id": "",
			},
			tenant: model.NewTenant{},

			checker: mt.NewJSONResponse(
				http.StatusBadRequest,
				nil,
				restError("TenantID: non zero value required;"),
			),
		},
		"error: empty json": {
			tenant: model.NewTenant{},

			checker: mt.NewJSONResponse(
				http.StatusBadRequest,
				nil,
				restError("JSON payload is empty"),
			),
		},
	}

	for name, tc := range testCases {
		t.Run(fmt.Sprintf("tc %s", name), func(t *testing.T) {

			ctx := contextMatcher()

			//make mock inventory
			inv := &minventory.InventoryApp{}
			inv.On("CreateTenant", ctx, tc.tenant).Return(tc.uaError)

			//make handler
			api := makeMockApiHandler(t, inv)

			//make request
			req := makeReq(http.MethodPost,
				"http://1.2.3.4/api/internal/v1/inventory/tenants",
				"",
				tc.body)

			//test
			recorded := test.RunRequest(t, api, req)
			mt.CheckResponse(t, tc.checker, recorded)
		})
	}
}

func makeReq(method, url, auth string, body interface{}) *http.Request {
	req := test.MakeSimpleRequest(method, url, body)

	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Add(requestid.RequestIdHeader, "test")

	return req
}

func restError(status string) map[string]interface{} {
	return map[string]interface{}{"error": status, "request_id": "test"}
}

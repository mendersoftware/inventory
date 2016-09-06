// Copyright 2016 Mender Software AS
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
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/ant0ine/go-json-rest/rest/test"
	"github.com/mendersoftware/inventory/config"
	"github.com/mendersoftware/inventory/log"
	"github.com/mendersoftware/inventory/requestid"
	"github.com/mendersoftware/inventory/requestlog"
	"github.com/mendersoftware/inventory/utils"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	. "github.com/stretchr/testify/mock"
	"net/http"
	"strconv"
	"testing"
)

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
	return map[string]interface{}{"error": status}
}

func makeMockApiHandler(t *testing.T, f InventoryFactory) http.Handler {
	handlers := NewInventoryApiHandlers(f)
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

func mockListDevices(num int) []Device {
	var devs []Device
	for i := 0; i < num; i++ {
		devs = append(devs, Device{ID: DeviceID(strconv.Itoa(i))})
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
		"valid pagination, no next page": {
			listDevicesNum: 5,
			listDevicesErr: nil,
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: utils.JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": []string{
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5", "first"),
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
					"Link": []string{
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5", "first"),
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
					"Link": []string{
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?attr_name1=qe%3A123%3A123%3A123&page=1&per_page=5", "first"),
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
					"Link": []string{
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&sort=attr_name1%3Aasc", "first"),
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
					"Link": []string{
						fmt.Sprintf(utils.LinkTmpl, "http://1.2.3.4/api/0.1.0/devices?has_group=true&page=1&per_page=5", "first"),
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
		inv := MockInventoryApp{}
		inv.On("ListDevices",
			AnythingOfType("int"),
			AnythingOfType("int"),
			AnythingOfType("[]main.Filter"),
			AnythingOfType("*main.Sort"),
			AnythingOfType("*bool"),
		).Return(mockListDevices(testCase.listDevicesNum), testCase.listDevicesErr)

		factory := func(c config.Reader, l *log.Logger) (InventoryApp, error) {
			return &inv, nil
		}
		apih := makeMockApiHandler(t, factory)

		recorded := test.RunRequest(t, apih, testCase.inReq)
		utils.CheckRecordedResponse(t, recorded, testCase.resp)
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
				"http://1.2.3.4/api/0.1.0/devices",
				nil),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
		},
		"garbled body": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/0.1.0/devices",
				"foo bar"),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type main.Device"),
			},
		},
		"body formatted ok, all fields present": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						map[string]interface{}{"name": "a1", "value": "00:00:00:01", "description": "ddd"},
						map[string]interface{}{"name": "a2", "value": 123.2, "description": "ddd"},
						map[string]interface{}{"name": "a3", "value": []interface{}{"00:00:00:01", "00"}, "description": "ddd"},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusCreated,
				OutputBodyObject: nil,
				OutputHeaders:    map[string][]string{"Location": []string{"http://1.2.3.4/api/0.1.0/devices/id-0001"}},
			},
		},
		"body formatted ok, wrong attributes type": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id":         "id-0001",
					"attributes": 123,
				},
			),
			inventoryErr: nil,
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal number into Go value of type []main.DeviceAttribute"),
			},
		},
		"body formatted ok, 'id' missing": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/0.1.0/devices",
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
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						map[string]interface{}{"name": "asd", "value": []interface{}{"asd", 123}},
						map[string]interface{}{"name": "asd2", "value": []interface{}{123, "asd"}},
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
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						map[string]interface{}{"value": "23"},
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
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						map[string]interface{}{
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
		"device ID already exists error": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/0.1.0/devices",
				map[string]interface{}{
					"id": "id-0001",
				},
			),
			inventoryErr: errors.Wrap(ErrDuplicatedDeviceId, "failed to add device"),
			JSONResponseParams: utils.JSONResponseParams{
				OutputStatus:     http.StatusConflict,
				OutputBodyObject: RestError("device with specified ID already exists"),
			},
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		inv := MockInventoryApp{}
		inv.On("AddDevice", AnythingOfType("*main.Device")).Return(tc.inventoryErr)

		factory := func(c config.Reader, l *log.Logger) (InventoryApp, error) {
			return &inv, nil
		}
		apih := makeMockApiHandler(t, factory)

		recorded := test.RunRequest(t, apih, tc.inReq)
		utils.CheckRecordedResponse(t, recorded, tc.JSONResponseParams)
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
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type []main.DeviceAttribute"),
			},
		},

		"body formatted ok, attribute name missing": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]DeviceAttribute{
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
				[]DeviceAttribute{
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
				[]DeviceAttribute{
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
				[]DeviceAttribute{
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
				[]DeviceAttribute{
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
		inv := MockInventoryApp{}

		inv.On("UpsertAttributes", AnythingOfType("main.DeviceID"), AnythingOfType("main.DeviceAttributes")).Return(tc.inventoryErr)

		factory := func(c config.Reader, l *log.Logger) (InventoryApp, error) {
			return &inv, nil
		}
		apih := makeMockApiHandler(t, factory)

		rest.ErrorFieldName = "error"

		for k, v := range tc.inHdrs {
			tc.inReq.Header.Set(k, v)
		}

		recorded := test.RunRequest(t, apih, tc.inReq)

		utils.CheckRecordedResponse(t, recorded, tc.resp)
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
				OutputBodyObject: RestError(ErrDevNotFound.Error()),
			},
			inventoryErr: ErrDevNotFound,
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
		inv := MockInventoryApp{}
		inv.On("UnsetDeviceGroup", AnythingOfType("main.DeviceID"), AnythingOfType("main.GroupName")).Return(tc.inventoryErr)

		factory := func(c config.Reader, l *log.Logger) (InventoryApp, error) {
			return &inv, nil
		}
		apih := makeMockApiHandler(t, factory)

		recorded := test.RunRequest(t, apih, tc.inReq)
		utils.CheckRecordedResponse(t, recorded, tc.JSONResponseParams)
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
				InventoryApiGroup{"abc"}),
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
				OutputBodyObject: RestError(ErrDevNotFound.Error()),
			},
			inventoryErr: ErrDevNotFound,
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
		inv := MockInventoryApp{}
		inv.On("UpdateDeviceGroup", AnythingOfType("main.DeviceID"), AnythingOfType("main.GroupName")).Return(tc.inventoryErr)

		factory := func(c config.Reader, l *log.Logger) (InventoryApp, error) {
			return &inv, nil
		}
		apih := makeMockApiHandler(t, factory)

		recorded := test.RunRequest(t, apih, tc.inReq)
		utils.CheckRecordedResponse(t, recorded, tc.JSONResponseParams)
	}
}

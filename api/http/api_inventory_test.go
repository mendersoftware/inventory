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
package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/ant0ine/go-json-rest/rest/test"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/mendersoftware/go-lib-micro/mongo/oid"
	"github.com/mendersoftware/go-lib-micro/requestid"
	"github.com/mendersoftware/go-lib-micro/requestlog"
	"github.com/mendersoftware/go-lib-micro/rest_utils"
	mt "github.com/mendersoftware/go-lib-micro/testing"

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

func runTestRequest(t *testing.T, handler http.Handler, req *http.Request, resp JSONResponseParams) {
	req.Header.Add(requestid.RequestIdHeader, "test")
	recorded := test.RunRequest(t, handler, req)
	CheckRecordedResponse(t, recorded, resp)
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

func floatPtr(f float64) *float64 {
	ret := f
	return &ret
}

func timePtr(f string) *time.Time {
	ret, _ := time.Parse("2006-01-02T15:04:05Z", f)
	return &ret
}

func TestLiveliness(t *testing.T) {
	api := makeMockApiHandler(t, nil)
	req, _ := http.NewRequest("GET", "http://localhost"+uriInternalAlive, nil)
	recorded := test.RunRequest(t, api, req)
	recorded.CodeIs(http.StatusNoContent)
}

func TestHealthCheck(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name     string
		AppError error
		HTTPCode int
	}{{
		Name:     "ok",
		HTTPCode: http.StatusNoContent,
	}, {
		Name:     "error, MongoDB not reachable",
		HTTPCode: http.StatusServiceUnavailable,
		AppError: errors.New("connection error"),
	}}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			app := &minventory.InventoryApp{}
			app.On("HealthCheck", mock.MatchedBy(
				func(ctx interface{}) bool {
					if _, ok := ctx.(context.Context); ok {
						return true
					}
					return false
				},
			)).Return(tc.AppError)
			req, _ := http.NewRequest(
				"GET",
				"http://localhost"+uriInternalHealth,
				nil,
			)
			req.Header.Add("X-MEN-RequestID", "test")
			api := makeMockApiHandler(t, app)
			recorded := test.RunRequest(t, api, req)
			recorded.CodeIs(tc.HTTPCode)
			if tc.HTTPCode == 204 {
				recorded.BodyIs("")
			} else {
				apiErr := rest_utils.ApiError{
					Err:   tc.AppError.Error(),
					ReqId: "test",
				}
				b, _ := json.Marshal(apiErr)
				assert.JSONEq(t,
					string(b),
					recorded.Recorder.Body.String(),
				)
			}
		})
	}
}

func TestApiParseFilterParams(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inReq   *http.Request
		filters []store.Filter
		err     error
	}{

		"eq - short form(implicit)": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=A0001", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "A0001",
					Operator:  store.Eq,
				},
			},
		},
		"eq - short form(implicit), colons": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=qe:123:123:123", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "qe:123:123:123",
					Operator:  store.Eq,
				},
			},
		},
		"eq - short form(implicit), float": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=3.14", nil),
			filters: []store.Filter{
				{
					AttrName:   "attr_name1",
					AttrScope:  model.AttrScopeInventory,
					Value:      "3.14",
					ValueFloat: floatPtr(3.14),
					Operator:   store.Eq,
				},
			},
		},
		"eq - short form(implicit), time": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=2014-11-12T11:45:26.371Z", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "2014-11-12T11:45:26.371Z",
					ValueTime: timePtr("2014-11-12T11:45:26.371Z"),
					Operator:  store.Eq,
				},
			},
		},
		"eq - short form(implicit), time without milliseconds": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=2014-11-12T11:45:26Z", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "2014-11-12T11:45:26Z",
					ValueTime: timePtr("2014-11-12T11:45:26Z"),
					Operator:  store.Eq,
				},
			},
		},
		"eq - long form": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=eq:A0001", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "A0001",
					Operator:  store.Eq,
				},
			},
		},
		"eq - long form, colons": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=eq:qe:123:123:123", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: model.AttrScopeInventory,
					Value:     "qe:123:123:123",
					Operator:  store.Eq,
				},
			},
		},
		"eq - long form, colons, with scope": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&scope/attr_name1=eq:qe:123:123:123", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr_name1",
					AttrScope: "scope",
					Value:     "qe:123:123:123",
					Operator:  store.Eq,
				},
			},
		},
		"eq - long form, dashes, with scope": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&scope/attr-name1=eq:qe-123-123-123", nil),
			filters: []store.Filter{
				{
					AttrName:  "attr-name1",
					AttrScope: "scope",
					Value:     "qe-123-123-123",
					Operator:  store.Eq,
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Run(fmt.Sprintf("tc %s", name), func(t *testing.T) {
			req := rest.Request{Request: testCase.inReq}
			filters, err := parseFilterParams(&req)
			if testCase.err != nil {
				assert.Error(t, testCase.err, err.Error())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, filters)
				assert.NotEmpty(t, filters)

				assert.ElementsMatch(t, testCase.filters, filters)
			}
		})
	}
}

func TestApiInventoryGetDevices(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		listDevicesNum  int
		listDevicesErr  error
		listDeviceTotal int
		inReq           *http.Request
		resp            JSONResponseParams
	}{
		"get all devices in group": {
			listDevicesNum:  3,
			listDevicesErr:  nil,
			listDeviceTotal: 18,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5&group=foo", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(3),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "group=foo&page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "group=foo&page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"18"},
				},
			},
		},
		"valid pagination, no next page": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 20,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"20"},
				},
			},
		},
		"valid pagination, with next page": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"21"},
				},
			},
		},
		"invalid pagination - page format": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=foo&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - per_page format": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=foo", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("per_page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - bounds": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=0&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmLimit("page")),
				OutputHeaders:    nil,
			},
		},
		"valid attribute filter": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&attr_name1=qe:123:123:123", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "attr_name1=qe%3A123%3A123%3A123&page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"5"},
				},
			},
		},
		"valid sort order value": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?sort=attr_name1:asc&page=1&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5&sort=attr_name1%3Aasc", "first"),
					},
					hdrTotalCount: {"5"},
				},
			},
		},
		"invalid sort order value": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&sort=attr_name1:gte", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("invalid sort order"),
				OutputHeaders:    nil,
			},
		},
		"valid has_group": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?has_group=true&page=1&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "has_group=true&page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"5"},
				},
			},
		},
		"invalid has_group": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 5,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=1&per_page=5&has_group=asd", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("has_group")),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevices error": {
			listDevicesNum:  5,
			listDevicesErr:  errors.New("inventory error"),
			listDeviceTotal: 20,
			inReq:           test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
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
		).Return(mockListDevices(testCase.listDevicesNum), testCase.listDeviceTotal, testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
	}
}

func TestApiInventoryAddDevice(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		JSONResponseParams

		inReq *http.Request

		inventoryErr error

		deviceAttributes model.DeviceAttributes
	}{
		"empty body": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				nil),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
		},
		"garbled body": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				"foo bar"),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type model.Device"),
			},
		},
		"body formatted ok, all fields present": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
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
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusCreated,
				OutputBodyObject: nil,
				OutputHeaders:    map[string][]string{"Location": {"devices/id-0001"}},
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "a1", Value: "00:00:00:01", Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
				{Name: "a2", Value: 123.2, Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
				{Name: "a3", Value: []interface{}{"00:00:00:01", "00"}, Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
			},
		},
		"body formatted ok, all fields present, attributes with scope": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"name": "a1", "value": "00:00:00:01", "description": "ddd", "scope": model.AttrScopeInventory},
						{"name": "a2", "value": 123.2, "description": "ddd", "scope": model.AttrScopeInventory},
						{"name": "a3", "value": []interface{}{"00:00:00:01", "00"}, "description": "ddd", "scope": model.AttrScopeInventory},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusCreated,
				OutputBodyObject: nil,
				OutputHeaders:    map[string][]string{"Location": {"devices/id-0001"}},
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "a1", Value: "00:00:00:01", Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
				{Name: "a2", Value: 123.2, Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
				{Name: "a3", Value: []interface{}{"00:00:00:01", "00"}, Description: strPtr("ddd"), Scope: model.AttrScopeInventory},
			},
		},
		"body formatted ok, wrong attributes type": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				map[string]interface{}{
					"id":         "id-0001",
					"attributes": 123,
				},
			),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal number into Go struct field Device.attributes of type []model.DeviceAttribute"),
			},
		},
		"body formatted ok, 'id' missing": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				map[string]interface{}{},
			),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("id: cannot be blank."),
			},
		},
		"body formatted ok, incorrect attribute value": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"name": "asd", "value": []interface{}{"asd", 123}},
						{"name": "asd2", "value": []interface{}{123, "asd"}},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("attributes: (value: array values must be of consistent type: string or float64.)."),
			},
		},
		"body formatted ok, attribute name missing": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
				map[string]interface{}{
					"id": "id-0001",
					"attributes": []map[string]interface{}{
						{"value": "23"},
					},
				},
			),
			inventoryErr: nil,
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("attributes: (name: cannot be blank.)."),
			},
		},
		"body formatted ok, inv error": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices",
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
			JSONResponseParams: JSONResponseParams{
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
			mock.MatchedBy(
				func(dev *model.Device) bool {
					if tc.deviceAttributes != nil {
						if !reflect.DeepEqual(tc.deviceAttributes, dev.Attributes) {
							assert.FailNow(t, "", "attributes not equal: %v \n%v\n", tc.deviceAttributes, dev.Attributes)
						}
					}
					return true
				},
			),
		).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
	}
}

func TestApiInventoryUpdateDeviceTags(t *testing.T) {

	testCases := map[string]struct {
		inReq         *http.Request
		inHdrs        map[string]string
		deviceID      model.DeviceID
		attrsToUpsert model.DeviceAttributes
		scope         string
		etag          string
		inventoryErr  error
		resp          JSONResponseParams
	}{
		"Replace tags, PUT, failed ETag": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			inHdrs: map[string]string{
				"If-Match": "f7238315-062d-4440-875a-676006f84c34",
			},
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: inventory.ErrETagDoesntMatch,
			etag:         "f7238315-062d-4440-875a-676006f84c34",
			resp: JSONResponseParams{
				OutputStatus:     http.StatusPreconditionFailed,
				OutputBodyObject: RestError("ETag does not match"),
			},
		},
		"ok, replace tags, PUT, with ETag": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			inHdrs: map[string]string{
				"If-Match": "f7238315-062d-4440-875a-676006f84c34",
			},
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: nil,
			etag:         "f7238315-062d-4440-875a-676006f84c34",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
		},
		"ok, replace tags, PUT, without ETag": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: nil,
			etag:         "",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
		},
		"Upsert tags, PATCH, failed ETag": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			inHdrs: map[string]string{
				"If-Match": "f7238315-062d-4440-875a-676006f84c34",
			},
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: inventory.ErrETagDoesntMatch,
			etag:         "f7238315-062d-4440-875a-676006f84c34",
			resp: JSONResponseParams{
				OutputStatus:     http.StatusPreconditionFailed,
				OutputBodyObject: RestError("ETag does not match"),
			},
		},
		"ok, upsert tags, PATCH, with ETag": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			inHdrs: map[string]string{
				"If-Match": "f7238315-062d-4440-875a-676006f84c34",
			},
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: nil,
			etag:         "f7238315-062d-4440-875a-676006f84c34",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
		},
		"ok, upsert tags, PATCH, without ETag": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/devices/:id/tags",
				[]model.DeviceAttribute{
					{
						Name:  "tag_1",
						Value: "value_1",
					},
					{
						Name:  "tag_2",
						Value: "value_2",
					},
				},
			),
			deviceID: "ad22a170-37b5-4c8b-9eab-612bad1eac19",
			attrsToUpsert: model.DeviceAttributes{
				{Name: "tag_1", Value: "value_1", Scope: model.AttrScopeTags},
				{Name: "tag_2", Value: "value_2", Scope: model.AttrScopeTags},
			},
			scope:        model.AttrScopeTags,
			inventoryErr: nil,
			etag:         "",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}
		ctx := contextMatcher()

		if tc.inReq.Method == http.MethodPatch {
			inv.On("UpsertAttributesWithUpdated",
				ctx,
				tc.deviceID,
				mock.MatchedBy(
					func(attrs model.DeviceAttributes) bool {
						if tc.attrsToUpsert != nil {
							for k, _ := range tc.attrsToUpsert {
								assert.Equal(t, tc.attrsToUpsert[k].Name, attrs[k].Name)
								assert.Equal(t, tc.attrsToUpsert[k].Value, attrs[k].Value)
								assert.Equal(t, tc.attrsToUpsert[k].Description, attrs[k].Description)
								assert.Equal(t, tc.attrsToUpsert[k].Scope, attrs[k].Scope)
							}
						}
						return true
					},
				),
				tc.scope,
				tc.etag,
			).Return(tc.inventoryErr)
		} else {
			inv.On("ReplaceAttributes",
				ctx,
				tc.deviceID,
				mock.MatchedBy(
					func(attrs model.DeviceAttributes) bool {
						if tc.attrsToUpsert != nil {
							for k, _ := range tc.attrsToUpsert {
								assert.Equal(t, tc.attrsToUpsert[k].Name, attrs[k].Name)
								assert.Equal(t, tc.attrsToUpsert[k].Value, attrs[k].Value)
								assert.Equal(t, tc.attrsToUpsert[k].Description, attrs[k].Description)
								assert.Equal(t, tc.attrsToUpsert[k].Scope, attrs[k].Scope)
							}
						}
						return true
					},
				),
				tc.scope,
				tc.etag,
			).Return(tc.inventoryErr)
		}

		apih := makeMockApiHandler(t, &inv)

		if tc.inHdrs != nil {
			for k, v := range tc.inHdrs {
				tc.inReq.Header.Set(k, v)
			}
		}

		tc.inReq.URL.Path = strings.Replace(tc.inReq.URL.Path, ":id", tc.deviceID.String(), -1)

		runTestRequest(t, apih, tc.inReq, tc.resp)

	}
}

func TestApiInventoryUpsertAttributes(t *testing.T) {
	t.Parallel()

	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		inReq  *http.Request
		inHdrs map[string]string

		scope string
		etag  string

		inventoryErr error

		resp             JSONResponseParams
		deviceAttributes model.DeviceAttributes
	}{
		"no auth": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusUnauthorized,
				OutputBodyObject: RestError("unauthorized"),
			},
			scope: model.AttrScopeInventory,
		},

		"invalid auth": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inHdrs: map[string]string{
				"Authorization:": "foobar",
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusUnauthorized,
				OutputBodyObject: RestError("unauthorized"),
			},
			scope: model.AttrScopeInventory,
		},

		"empty body": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				nil),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
			scope: model.AttrScopeInventory,
		},

		"garbled body": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				`{"foo": "bar"}`),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type []model.DeviceAttribute"),
			},
			scope: model.AttrScopeInventory,
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("name: cannot be blank."),
			},
			scope: model.AttrScopeInventory,
		},

		"body formatted ok, attribute value missing": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Description: strPtr("descr1"),
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("value: supported types are string, float64, and arrays thereof."),
			},
			scope: model.AttrScopeInventory,
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "name1", Value: "value1", Description: strPtr("descr1"), Scope: model.AttrScopeInventory},
				{Name: "name2", Value: float64(2), Description: strPtr("descr2"), Scope: model.AttrScopeInventory},
			},
			scope: model.AttrScopeInventory,
		},

		"body formatted ok, attributes ok (all fields), with scope": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:        "name1",
						Value:       "value1",
						Description: strPtr("descr1"),
						Scope:       model.AttrScopeInventory,
					},
					{
						Name:        "name2",
						Value:       2,
						Description: strPtr("descr2"),
						Scope:       model.AttrScopeInventory,
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "name1", Value: "value1", Description: strPtr("descr1"), Scope: model.AttrScopeInventory},
				{Name: "name2", Value: float64(2), Description: strPtr("descr2"), Scope: model.AttrScopeInventory},
			},
			scope: model.AttrScopeInventory,
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			scope: model.AttrScopeInventory,
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			scope: model.AttrScopeInventory,
		},

		"body formatted ok, attributes ok, but values are empty": {
			inReq: test.MakeSimpleRequest("PATCH",
				"http://1.2.3.4/api/0.1.0/attributes",
				[]model.DeviceAttribute{
					{
						Name:  "name1",
						Value: "",
					},
					{
						Name:  "name2",
						Value: "",
					},
				},
			),
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			scope: model.AttrScopeInventory,
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			scope: model.AttrScopeInventory,
		},

		"body formatted ok, attributes ok (values only), PUT": {
			inReq: test.MakeSimpleRequest("PUT",
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
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			scope: model.AttrScopeInventory,
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		if tc.inReq.Method == http.MethodPatch {
			inv.On("UpsertAttributesWithUpdated",
				ctx,
				mock.AnythingOfType("model.DeviceID"),
				mock.MatchedBy(
					func(attrs model.DeviceAttributes) bool {
						if tc.deviceAttributes != nil {
							if !reflect.DeepEqual(tc.deviceAttributes, attrs) {
								assert.FailNow(t, "", "attributes not equal: %v \n%v\n", tc.deviceAttributes, attrs)
							}
						}
						return true
					},
				),
				tc.scope,
				tc.etag,
			).Return(tc.inventoryErr)
		} else {
			inv.On("ReplaceAttributes",
				ctx,
				mock.AnythingOfType("model.DeviceID"),
				mock.MatchedBy(
					func(attrs model.DeviceAttributes) bool {
						if tc.deviceAttributes != nil {
							if !reflect.DeepEqual(tc.deviceAttributes, attrs) {
								assert.FailNow(t, "", "attributes not equal: %v \n%v\n", tc.deviceAttributes, attrs)
							}
						}
						return true
					},
				),
				tc.scope,
				tc.etag,
			).Return(tc.inventoryErr)
		}

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

func TestApiInventoryUpsertAttributesInternal(t *testing.T) {
	t.Parallel()

	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		tenantId string
		deviceId string
		scope    string
		inReq    *http.Request
		inHdrs   map[string]string
		payload  interface{}

		inventoryErr error

		resp             JSONResponseParams
		deviceAttributes model.DeviceAttributes
	}{
		"empty body": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: JSON payload is empty"),
			},
		},

		"missing device id": {
			tenantId: "3456355",
			scope:    "inventory",
			payload: []model.DeviceAttribute{
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

			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("device id cannot be empty"),
			},
		},

		"garbled body": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: `{"foo": "bar"}`,
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode request body: json: cannot unmarshal string into Go value of type []model.DeviceAttribute"),
			},
		},

		"body formatted ok, attribute name missing": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
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
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("name: cannot be blank."),
			},
		},

		"body formatted ok, attribute value missing": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
				{
					Name:        "name1",
					Description: strPtr("descr1"),
				},
			},
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("value: supported types are string, float64, and arrays thereof."),
			},
		},

		"body formatted ok, attributes ok (all fields)": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
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
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "name1", Value: "value1", Description: strPtr("descr1"), Scope: model.AttrScopeInventory},
				{Name: "name2", Value: float64(2), Description: strPtr("descr2"), Scope: model.AttrScopeInventory},
			},
		},

		"body formatted ok, attributes ok (all fields), with scope": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
				{
					Name:        "name1",
					Value:       "value1",
					Description: strPtr("descr1"),
					Scope:       model.AttrScopeInventory,
				},
				{
					Name:        "name2",
					Value:       2,
					Description: strPtr("descr2"),
					Scope:       model.AttrScopeInventory,
				},
			},
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
			deviceAttributes: model.DeviceAttributes{
				{Name: "name1", Value: "value1", Description: strPtr("descr1"), Scope: model.AttrScopeInventory},
				{Name: "name2", Value: float64(2), Description: strPtr("descr2"), Scope: model.AttrScopeInventory},
			},
		},

		"body formatted ok, attributes ok (all fields, arrays)": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
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
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok (values only)": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
				{
					Name:  "name1",
					Value: "value1",
				},
				{
					Name:  "name2",
					Value: 2,
				},
			},
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok, but values are empty": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
				{
					Name:  "name1",
					Value: "",
				},
				{
					Name:  "name2",
					Value: "",
				},
			},
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: nil,
			},
		},

		"body formatted ok, attributes ok (all fields), inventory err": {
			tenantId: "3456355",
			deviceId: "sdfg435fgs-gs-dgsfgdfs-3456dgsf",
			scope:    "inventory",

			payload: []model.DeviceAttribute{
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
			inHdrs: map[string]string{
				"Authorization": makeDeviceAuthHeader(`{"sub": "fakeid"}`),
			},
			inventoryErr: errors.New("internal error"),
			resp: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
	}

	for name, tc := range testCases {
		t.Logf("test case: %s", name)
		tc.inReq = test.MakeSimpleRequest("PATCH",
			"http://1.2.3.4/api/internal/v1/inventory/tenants/"+tc.tenantId+"/device/"+tc.deviceId+"/attribute/scope/"+tc.scope,
			tc.payload)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("UpsertAttributes",
			ctx,
			mock.AnythingOfType("model.DeviceID"),
			mock.MatchedBy(
				func(attrs model.DeviceAttributes) bool {
					if tc.deviceAttributes != nil {
						if !reflect.DeepEqual(tc.deviceAttributes, attrs) {
							assert.FailNow(t, "", "attributes not equal: %v \n%v\n", tc.deviceAttributes, attrs)
						}
					}
					return true
				},
			),
		).Return(tc.inventoryErr)

		apih := makeMockApiHandler(t, &inv)

		rest.ErrorFieldName = "error"

		for k, v := range tc.inHdrs {
			tc.inReq.Header.Set(k, v)
		}

		runTestRequest(t, apih, tc.inReq, tc.resp)
	}
}

func TestApiInventoryDeleteDeviceGroup(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

		inReq *http.Request

		inventoryErr error
	}{
		"ok": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNoContent,
				OutputBodyObject: nil,
			},
		},
		"device group not found (or device's group is other than requested)": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
			inventoryErr: store.ErrDevNotFound,
		},
		"internal error": {
			inReq: test.MakeSimpleRequest("DELETE",
				"http://1.2.3.4/api/0.1.0/devices/123/group/g1", nil),
			JSONResponseParams: JSONResponseParams{
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
		JSONResponseParams

		inReq *http.Request

		inventoryErr error
	}{
		"ok": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"_a-b-c_"}),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNoContent,
				OutputBodyObject: nil,
			},
		},
		"device not found": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"abc"}),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
			inventoryErr: store.ErrDevNotFound,
		},
		"empty group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{}),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group name cannot be blank"),
			},
			inventoryErr: nil,
		},
		"unsupported characters in group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"__+X@#$  ;"}),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group name can only contain: upper/lowercase alphanum, -(dash), _(underscore)"),
			},
			inventoryErr: nil,
		},
		"non-ASCII characters in group name": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"ęą"}),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("Group name can only contain: upper/lowercase alphanum, -(dash), _(underscore)"),
			},
			inventoryErr: nil,
		},
		"empty body": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("failed to decode device group data: JSON payload is empty"),
			},
			inventoryErr: nil,
		},
		"internal error": {
			inReq: test.MakeSimpleRequest("PUT",
				"http://1.2.3.4/api/0.1.0/devices/123/group",
				InventoryApiGroup{"abc"}),
			JSONResponseParams: JSONResponseParams{
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
		JSONResponseParams

		inReq        *http.Request
		outputGroups []model.GroupName

		inventoryErr error
	}{
		"some groups": {
			inReq:        test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups", nil),
			outputGroups: []model.GroupName{"foo", "bar"},
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: []string{"foo", "bar"},
			},
		},
		"no groups": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups?status=rejected", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: []string{},
			},
		},
		"error": {
			inReq: test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("internal error"),
		},
	}

	for name, tc := range tcases {
		t.Run(name, func(t *testing.T) {
			var filters []model.FilterPredicate
			if status := tc.inReq.URL.Query().
				Get("status"); status != "" {
				filters = []model.FilterPredicate{{
					Scope:     model.AttrScopeIdentity,
					Attribute: "status",
					Type:      "$eq",
					Value:     status,
				}}
			}
			inv := minventory.InventoryApp{}
			ctx := contextMatcher()

			inv.On("ListGroups", ctx, filters).
				Return(tc.outputGroups, tc.inventoryErr)

			apih := makeMockApiHandler(t, &inv)

			runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
		})
	}
}

func TestApiGetDevice(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

		inReq        *http.Request
		inDevId      model.DeviceID
		outputDevice *model.Device
		inventoryErr error
	}{
		"no device": {
			inDevId:      model.DeviceID("1"),
			inReq:        test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1", nil),
			outputDevice: nil,
			JSONResponseParams: JSONResponseParams{
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
			JSONResponseParams: JSONResponseParams{
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
			JSONResponseParams: JSONResponseParams{
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
		listDevicesNum   int
		listDevicesErr   error
		listDevicesTotal int
		inReq            *http.Request
		resp             JSONResponseParams
	}{
		"valid pagination, no next page": {
			listDevicesNum:   5,
			listDevicesErr:   nil,
			listDevicesTotal: 20,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDeviceIDs(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"20"},
				},
			},
		},
		"valid pagination, with next page": {
			listDevicesNum:   5,
			listDevicesErr:   nil,
			listDevicesTotal: 21,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDeviceIDs(5),
				OutputHeaders: map[string][]string{
					"Link": {
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=3&per_page=5", "prev"),
						fmt.Sprintf(utils.LinkTmpl, "devices", "page=1&per_page=5", "first"),
					},
					hdrTotalCount: {"21"},
				},
			},
		},
		"invalid pagination - page format": {
			listDevicesNum:   5,
			listDevicesErr:   nil,
			listDevicesTotal: 5,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=foo&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - per_page format": {
			listDevicesNum:   5,
			listDevicesErr:   nil,
			listDevicesTotal: 5,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=1&per_page=foo", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmInvalid("per_page")),
				OutputHeaders:    nil,
			},
		},
		"invalid pagination - bounds": {
			listDevicesNum:   5,
			listDevicesErr:   nil,
			listDevicesTotal: 5,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=0&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError(utils.MsgQueryParmLimit("page")),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevicesByGroup error - group not found": {
			listDevicesNum:   5,
			listDevicesErr:   store.ErrGroupNotFound,
			listDevicesTotal: 20,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
				OutputStatus:     404,
				OutputBodyObject: RestError("group not found"),
				OutputHeaders:    nil,
			},
		},
		"inv.ListDevicesByGroup error - internal": {
			listDevicesNum:   5,
			listDevicesErr:   errors.New("inventory error"),
			listDevicesTotal: 20,
			inReq:            test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/groups/foo/devices?page=4&per_page=5", nil),
			resp: JSONResponseParams{
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
		).Return(mockListDeviceIDs(testCase.listDevicesNum), testCase.listDevicesTotal, testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
	}
}

func TestApiGetDeviceGroup(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

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

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: map[string]interface{}{"group": "dev"},
			},
		},
		"device without group": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   nil,

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: map[string]interface{}{"group": nil},
			},
		},
		"device not found": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   store.ErrDevNotFound,

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
		},
		"generic inventory error": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/0.1.0/devices/1/group", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   errors.New("inventory: internal error"),

			JSONResponseParams: JSONResponseParams{
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

func TestApiGetDeviceGroupInternal(t *testing.T) {
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

		inReq *http.Request

		inventoryGroup model.GroupName
		inventoryErr   error
	}{
		"device with group": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/internal/v1/inventory/tenants/foo/devices/1/groups", nil),
			inventoryGroup: model.GroupName("dev"),
			inventoryErr:   nil,

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: model.DeviceGroups{Groups: []string{"dev"}},
			},
		},
		"device without group": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/internal/v1/inventory/tenants/foo/devices/1/groups", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   nil,

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusOK,
				OutputBodyObject: model.DeviceGroups{},
			},
		},
		"device not found": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/internal/v1/inventory/tenants/foo/devices/1/groups", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   store.ErrDevNotFound,

			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError(store.ErrDevNotFound.Error()),
			},
		},
		"generic inventory error": {
			inReq:          test.MakeSimpleRequest("GET", "http://1.2.3.4/api/internal/v1/inventory/tenants/foo/devices/1/groups", nil),
			inventoryGroup: model.GroupName(""),
			inventoryErr:   errors.New("inventory: internal error"),

			JSONResponseParams: JSONResponseParams{
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

func TestApiDeleteDeviceInventory(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

		inReq        *http.Request
		inDevId      model.DeviceID
		inventoryErr error
	}{
		"no device": {
			inDevId:      model.DeviceID("1"),
			inReq:        test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/1", nil),
			inventoryErr: store.ErrDevNotFound,
			JSONResponseParams: JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"some device": {
			inDevId: model.DeviceID("2"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/2", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"error": {
			inDevId: model.DeviceID("3"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/0.1.0/devices/3", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("inventory internal error"),
		},
	}

	for name, tc := range tcases {
		t.Run(fmt.Sprintf("test case: %s", name), func(t *testing.T) {

			inv := minventory.InventoryApp{}

			ctx := contextMatcher()

			inv.On("ReplaceAttributes",
				ctx,
				tc.inDevId,
				model.DeviceAttributes{},
				model.AttrScopeInventory,
				"").Return(tc.inventoryErr)

			apih := makeMockApiHandler(t, &inv)

			runTestRequest(t, apih, tc.inReq, tc.JSONResponseParams)
		})
	}
}

func TestApiDeleteDevice(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	tcases := map[string]struct {
		JSONResponseParams

		inReq        *http.Request
		inDevId      model.DeviceID
		inventoryErr error
	}{
		"no device": {
			inDevId:      model.DeviceID("1"),
			inReq:        test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices/1", nil),
			inventoryErr: store.ErrDevNotFound,
			JSONResponseParams: JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"some device": {
			inDevId: model.DeviceID("2"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices/2", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus: http.StatusNoContent,
			},
		},
		"error": {
			inDevId: model.DeviceID("3"),
			inReq:   test.MakeSimpleRequest("DELETE", "http://1.2.3.4/api/internal/v1/inventory/tenants/1/devices/3", nil),
			JSONResponseParams: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			inventoryErr: errors.New("inventory internal error"),
		},
	}

	for name, tc := range tcases {
		t.Run(fmt.Sprintf("test case: %s", name), func(t *testing.T) {

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

func TestAPIClearDevicesGroup(t *testing.T) {
	testCases := []struct {
		Name string

		Devices []model.DeviceID
		model.GroupName
		*http.Request
		JSONResponseParams
		InventoryErr error
	}{{
		Name: "ok, some devices",

		Request: test.MakeSimpleRequest(
			"DELETE",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{"1", "2", "3"},
		),
		GroupName: "foo",
		Devices:   []model.DeviceID{"1", "2", "3"},
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusOK,
			OutputBodyObject: &model.UpdateResult{
				UpdatedCount: 3,
			},
		},
	}, {
		Name: "error, empty device list",

		Request: test.MakeSimpleRequest(
			"DELETE",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{},
		),
		Devices:   []model.DeviceID{},
		GroupName: "foo",
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error":      "no device IDs present in payload",
				"request_id": "test",
			},
		},
	}, {
		Name: "error, invalid schema",

		Request: test.MakeSimpleRequest(
			"DELETE",
			"http://localhost/api/0.1.0/groups/foo/devices",
			map[string]string{"foo": "bar"},
		),
		GroupName: "foo",
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error": "invalid payload schema: json: " +
					"cannot unmarshal object into Go " +
					"value of type []model.DeviceID",
				"request_id": "test",
			},
		},
	}, {
		Name: "internal error",

		Request: test.MakeSimpleRequest(
			"DELETE",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{"1", "2", "3"},
		),
		GroupName: "foo",
		Devices:   []model.DeviceID{"1", "2", "3"},
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusInternalServerError,
			OutputBodyObject: map[string]interface{}{
				"error":      "internal error",
				"request_id": "test",
			},
		},
		InventoryErr: errors.New("unknown error"),
	}, {
		Name: "error, invalid group name",

		Request: test.MakeSimpleRequest(
			"DELETE",
			"http://localhost/api/0.1.0/groups/illegal$group$name/devices",
			[]model.DeviceID{"1", "2", "3"},
		),
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error": "Group name can only contain: upper/lowercase " +
					"alphanum, -(dash), _(underscore)",
				"request_id": "test",
			},
		},
	}}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			inv := minventory.InventoryApp{}
			ctx := contextMatcher()
			apih := makeMockApiHandler(t, &inv)

			var ret *model.UpdateResult
			if rsp, ok := testCase.JSONResponseParams.
				OutputBodyObject.(*model.
				UpdateResult); ok {
				ret = rsp
			}
			inv.On("UnsetDevicesGroup",
				ctx,
				testCase.Devices,
				testCase.GroupName,
			).Return(
				ret,
				testCase.InventoryErr,
			)
			runTestRequest(t, apih,
				testCase.Request,
				testCase.JSONResponseParams,
			)
		})
	}
}

func TestAPIPatchGroupDevices(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		Name string

		Devices []model.DeviceID
		model.GroupName
		InventoryErr error

		*http.Request
		JSONResponseParams
	}{{
		Name: "ok, all device IDs match",

		Request: test.MakeSimpleRequest(
			"PATCH",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{"1", "2", "3"},
		),
		Devices:   []model.DeviceID{"1", "2", "3"},
		GroupName: "foo",
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusOK,
			OutputBodyObject: &model.UpdateResult{
				MatchedCount: 3,
				UpdatedCount: 3,
			},
		},
	}, {
		Name: "error, invalid JSON schema",

		Request: test.MakeSimpleRequest(
			"PATCH",
			"http://localhost/api/0.1.0/groups/foo/devices",
			map[string][]string{"devices": {"foo", "bar", "baz"}},
		),
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error": "invalid payload schema: json: " +
					"cannot unmarshal object into Go " +
					"value of type []model.DeviceID",
				"request_id": "test",
			},
		},
	}, {
		Name: "error, empty devices list",

		Request: test.MakeSimpleRequest(
			"PATCH",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{}),
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error":      "no device IDs present in payload",
				"request_id": "test",
			},
		},
	}, {
		Name: "error, invalid group name",

		Request: test.MakeSimpleRequest(
			"PATCH",
			"http://localhost/api/0.1.0/groups/deeeåååhh/devices",
			[]model.DeviceID{"1", "2"},
		),
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusBadRequest,
			OutputBodyObject: map[string]interface{}{
				"error": "Group name can only contain: " +
					"upper/lowercase alphanum, " +
					"-(dash), _(underscore)",
				"request_id": "test",
			},
		},
	}, {
		Name: "error, internal error",

		Request: test.MakeSimpleRequest(
			"PATCH",
			"http://localhost/api/0.1.0/groups/foo/devices",
			[]model.DeviceID{"1", "2"},
		),
		Devices:      []model.DeviceID{"1", "2"},
		GroupName:    "foo",
		InventoryErr: errors.New("unknown error"),
		JSONResponseParams: JSONResponseParams{
			OutputStatus: http.StatusInternalServerError,
			OutputBodyObject: map[string]interface{}{
				"error":      "internal error",
				"request_id": "test",
			},
		},
	}}

	for _, testCase := range testCases {
		t.Run(testCase.Name, func(t *testing.T) {
			inv := minventory.InventoryApp{}
			ctx := contextMatcher()
			apih := makeMockApiHandler(t, &inv)

			var ret *model.UpdateResult
			if rsp, ok := testCase.JSONResponseParams.
				OutputBodyObject.(*model.
				UpdateResult); ok {
				ret = rsp
			}
			inv.On("UpdateDevicesGroup",
				ctx,
				testCase.Devices,
				testCase.GroupName,
			).Return(
				ret,
				testCase.InventoryErr,
			)
			runTestRequest(t, apih,
				testCase.Request,
				testCase.JSONResponseParams,
			)
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
				restError("tenant_id: cannot be blank."),
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

func TestApiInventoryInternalDevicesStatus(t *testing.T) {
	t.Parallel()

	rest.ErrorFieldName = "error"

	tenantId := "5abcb6de7a673a0001287c71"
	emptyTenant := ""
	acceptedStatus := "accepted"

	testCases := map[string]struct {
		// Let intputDevices be interface{} type in order to allow
		// passing illegal request body values.
		//inputDevices []model.DeviceUpdate
		inputDevices interface{}
		tenantID     string
		status       string
		*model.UpdateResult

		callsInventory bool
		inventoryErr   error

		resp JSONResponseParams
	}{
		"ok": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("3").String()), Revision: 1},
			},
			tenantID: tenantId,
			status:   acceptedStatus,
			UpdateResult: &model.UpdateResult{
				MatchedCount: 3,
				UpdatedCount: 2,
			},
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
				OutputBodyObject: &model.UpdateResult{
					MatchedCount: 3,
					UpdatedCount: 2,
				},
			},
			callsInventory: true,
		},
		"ok, noauth": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("3").String()), Revision: 1},
			},
			tenantID: tenantId,
			status:   "noauth",
			UpdateResult: &model.UpdateResult{
				MatchedCount: 3,
				UpdatedCount: 2,
			},
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
				OutputBodyObject: &model.UpdateResult{
					MatchedCount: 3,
					UpdatedCount: 2,
				},
			},
			callsInventory: true,
		},

		"ok single tenant": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
			},
			tenantID: emptyTenant,
			status:   acceptedStatus,
			UpdateResult: &model.UpdateResult{
				MatchedCount: 2,
				UpdatedCount: 2,
			},
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
				OutputBodyObject: &model.UpdateResult{
					MatchedCount: 2,
					UpdatedCount: 2,
				},
			},
			callsInventory: true,
		},
		"error, payload empty": {
			tenantID:     tenantId,
			status:       acceptedStatus,
			inputDevices: nil,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("cant parse devices: JSON payload is empty"),
			},
			callsInventory: false,
		},

		"error, payload not expected": {
			inputDevices: "sneaky wool carpet",
			tenantID:     tenantId,
			status:       acceptedStatus,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("cant parse devices: json: cannot unmarshal string into Go value of type []model.DeviceUpdate"),
			},
			callsInventory: false,
		},

		"error, bad status": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
			},
			tenantID: emptyTenant,
			status:   "quo",
			UpdateResult: &model.UpdateResult{
				MatchedCount: 1,
				UpdatedCount: 1,
			},
			resp: JSONResponseParams{
				OutputStatus:     http.StatusNotFound,
				OutputBodyObject: RestError("unrecognized status: quo"),
			},
			callsInventory: false,
		},

		"error, db Upsert failed": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
			},
			tenantID:     tenantId,
			status:       acceptedStatus,
			inventoryErr: errors.New("cant upsert"),
			resp: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
			callsInventory: true,
		},
		"error, conflict": {
			inputDevices: []model.DeviceUpdate{
				{Id: model.DeviceID(oid.NewUUIDv5("1").String()), Revision: 1},
				{Id: model.DeviceID(oid.NewUUIDv5("2").String()), Revision: 1},
			},
			tenantID:     tenantId,
			status:       acceptedStatus,
			inventoryErr: store.ErrWriteConflict,
			resp: JSONResponseParams{
				OutputStatus:     http.StatusConflict,
				OutputBodyObject: RestError("write conflict"),
			},
			callsInventory: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var (
				inReq = test.MakeSimpleRequest("POST",
					"http://1.2.3.4/api/internal/v1/inventory/tenants/"+
						tc.tenantID+"/devices/status/"+tc.status,
					tc.inputDevices,
				)
				deviceAttributes = model.DeviceAttributes{{
					Name:  "status",
					Value: tc.status,
					Scope: model.AttrScopeIdentity,
				}}
			)

			inv := minventory.InventoryApp{}
			ctx := contextMatcher()

			if tc.callsInventory {
				switch tc.status {
				case "accepted", "preauthorized", "pending", "noauth":
					// Update statuses
					inv.On("UpsertDevicesStatuses",
						ctx,
						tc.inputDevices,
						deviceAttributes,
					).Return(tc.UpdateResult, tc.inventoryErr)

				case "decommissioned":
					// Delete Inventory
					inv.On("DeleteDevices",
						ctx,
						getIdsFromDevices(tc.inputDevices.([]model.DeviceUpdate)),
					).Return(tc.UpdateResult, tc.inventoryErr)
				}
			}

			apih := makeMockApiHandler(t, &inv)

			rest.ErrorFieldName = "error"

			runTestRequest(t, apih, inReq, tc.resp)

			inv.AssertExpectations(t)
		})
	}
}

func TestApiInventoryFiltersAttributes(t *testing.T) {
	testCases := map[string]struct {
		attributes []model.FilterAttribute
		err        error
		httpCode   int
	}{
		"ok": {
			attributes: []model.FilterAttribute{
				{
					Name:  "name",
					Scope: "scope",
					Count: 100,
				},
				{
					Name:  "other_name",
					Scope: "scope",
					Count: 90,
				},
			},
			httpCode: http.StatusOK,
		},
		"ko": {
			err:      errors.New("error"),
			httpCode: http.StatusInternalServerError,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			inv := minventory.InventoryApp{}
			inv.On("GetFiltersAttributes",
				contextMatcher(),
			).Return(tc.attributes, tc.err)

			api := makeMockApiHandler(t, &inv)
			req, _ := http.NewRequest("GET", "http://localhost"+urlFiltersAttributes, nil)
			recorded := test.RunRequest(t, api, req)

			recorded.CodeIs(tc.httpCode)
			if tc.httpCode == http.StatusOK {
				body, _ := json.Marshal(tc.attributes)
				recorded.BodyIs(string(body))
			}
		})
	}
}

func TestApiInventorySearchDevices(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		listDevicesNum  int
		listDevicesErr  error
		listDeviceTotal int
		inReq           *http.Request
		resp            JSONResponseParams
	}{
		"valid pagination, no next page": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 20,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"20"},
				},
			},
		},
		"valid pagination, with next page": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"21"},
				},
			},
		},
		"valid filter and sort": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"21"},
				},
			},
		},
		"invalid filter": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("attribute: cannot be blank; scope: cannot be blank; type: cannot be blank; value: is required."),
				OutputHeaders:    nil,
			},
		},
		"invalid sort": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("attribute: cannot be blank; order: cannot be blank; scope: cannot be blank."),
				OutputHeaders:    nil,
			},
		},
		"inventory error": {
			listDevicesNum:  5,
			listDevicesErr:  errors.New("inventory error"),
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     500,
				OutputBodyObject: RestError("internal error"),
				OutputHeaders:    nil,
			},
		},
		"inventory error, BadValue": {
			listDevicesNum:  5,
			listDevicesErr:  errors.New("inventory error: BadValue"),
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("inventory error: BadValue"),
				OutputHeaders:    nil,
			},
		},
		"valid": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"21"},
				},
			},
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("SearchDevices",
			ctx,
			mock.AnythingOfType("model.SearchParams"),
		).Return(mockListDevices(testCase.listDevicesNum), testCase.listDeviceTotal, testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
	}
}

func TestApiParseSearchParams(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		inReq        *http.Request
		searchParams *model.SearchParams
		err          error
	}{
		"ok": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			searchParams: &model.SearchParams{
				Page:    4,
				PerPage: 5,
				Filters: []model.FilterPredicate{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Type:      "$eq",
						Value:     "bar",
					},
					{
						Scope:     "inventory",
						Attribute: "foo1",
						Type:      "$eq",
						Value:     "baz",
					},
				},
				Sort: []model.SortCriteria{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Order:     "asc",
					},
					{
						Scope:     "inventory",
						Attribute: "foo1",
						Order:     "desc",
					},
				},
			},
		},
		"ok: all filter types and sort orders": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     5,
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			searchParams: &model.SearchParams{
				Page:    4,
				PerPage: 5,
				Filters: []model.FilterPredicate{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Type:      "$eq",
						Value:     "bar",
					},
					{
						Scope:     "inventory",
						Attribute: "foo1",
						Type:      "$eq",
						Value:     float64(5),
					},
				},
				Sort: []model.SortCriteria{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Order:     "asc",
					},
					{
						Scope:     "inventory",
						Attribute: "foo1",
						Order:     "desc",
					},
				},
			},
		},
		"invalid Page and perPage": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    -3,
					PerPage: 0,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
					},
				},
			),
			searchParams: &model.SearchParams{
				Page:    utils.PageDefault,
				PerPage: utils.PerPageDefault,
				Filters: []model.FilterPredicate{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Type:      "$eq",
						Value:     "bar",
					},
				},
				Sort: []model.SortCriteria{
					{
						Scope:     "inventory",
						Attribute: "foo",
						Order:     "asc",
					},
				},
			},
		},
		"wrong sort order": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "foo",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			err: errors.New("order: must be a valid value."),
		},
		"wrong filter type": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$neq",
							Value:     "baz",
						},
					},
				},
			),
			err: errors.New("type: must be a valid value."),
		},
		"invalid JSON": {
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/management/v2/inventory/filters/search",
				"invalid json",
			),
			err: errors.New("failed to decode request body: json: cannot unmarshal string into Go value of type model.SearchParams"),
		},
	}

	for name, tc := range testCases {
		t.Run(fmt.Sprintf("test case: %s", name), func(t *testing.T) {
			req := rest.Request{Request: tc.inReq}
			params, err := parseSearchParams(&req)
			if tc.err != nil {
				assert.EqualError(t, tc.err, err.Error())
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, params)
				assert.NotEmpty(t, params)

				assert.Equal(t, tc.searchParams, params)
			}
		})
	}
}

func TestApiInventoryInternalSearchDevices(t *testing.T) {
	t.Parallel()
	rest.ErrorFieldName = "error"

	testCases := map[string]struct {
		listDevicesNum  int
		listDevicesErr  error
		listDeviceTotal int
		inReq           *http.Request
		resp            JSONResponseParams
	}{
		"valid filter and sort": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v2/inventory/tenants/foo/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"21"},
				},
			},
		},
		"valid filter and sort no tenant": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v2/inventory/tenants//filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     200,
				OutputBodyObject: mockListDevices(5),
				OutputHeaders: map[string][]string{
					hdrTotalCount: {"21"},
				},
			},
		},
		"invalid filter": {
			listDevicesNum:  5,
			listDevicesErr:  nil,
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v2/inventory/tenants/foo/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("attribute: cannot be blank; scope: cannot be blank; type: cannot be blank; value: is required."),
				OutputHeaders:    nil,
			},
		},
		"inventory error": {
			listDevicesNum:  5,
			listDevicesErr:  errors.New("inventory error"),
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v2/inventory/tenants/foo/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     500,
				OutputBodyObject: RestError("internal error"),
				OutputHeaders:    nil,
			},
		},
		"inventory error, BadValue": {
			listDevicesNum:  5,
			listDevicesErr:  errors.New("inventory error: BadValue"),
			listDeviceTotal: 21,
			inReq: test.MakeSimpleRequest("POST",
				"http://1.2.3.4/api/internal/v2/inventory/tenants/foo/filters/search",
				model.SearchParams{
					Page:    4,
					PerPage: 5,
					Filters: []model.FilterPredicate{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Type:      "$eq",
							Value:     "bar",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Type:      "$eq",
							Value:     "baz",
						},
					},
					Sort: []model.SortCriteria{
						{
							Scope:     "inventory",
							Attribute: "foo",
							Order:     "asc",
						},
						{
							Scope:     "inventory",
							Attribute: "foo1",
							Order:     "desc",
						},
					},
				},
			),
			resp: JSONResponseParams{
				OutputStatus:     400,
				OutputBodyObject: RestError("inventory error: BadValue"),
				OutputHeaders:    nil,
			},
		},
	}

	for name, testCase := range testCases {
		t.Logf("test case: %s", name)
		inv := minventory.InventoryApp{}

		ctx := contextMatcher()

		inv.On("SearchDevices",
			ctx,
			mock.AnythingOfType("model.SearchParams"),
		).Return(mockListDevices(testCase.listDevicesNum), testCase.listDeviceTotal, testCase.listDevicesErr)

		apih := makeMockApiHandler(t, &inv)

		runTestRequest(t, apih, testCase.inReq, testCase.resp)
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

func TestApiInventoryInternalReindex(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		tenantID    string
		deviceID    string
		serviceName string

		callsUpsertAttributes bool
		upsertAttributesErr   error

		callsCheckAlerts bool
		alertsCount      int
		checkAlertsError error

		deviceAttributes model.DeviceAttributes

		resp JSONResponseParams
	}{
		"ok, alerts": {
			tenantID:    "foo",
			deviceID:    "bar",
			serviceName: "devicemonitor",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
			callsUpsertAttributes: true,
			callsCheckAlerts:      true,
			alertsCount:           3,
			deviceAttributes: model.DeviceAttributes{
				{Name: model.AttrNameNumberOfAlerts, Value: 3, Scope: model.AttrScopeMonitor},
				{Name: model.AttrNameAlerts, Value: true, Scope: model.AttrScopeMonitor},
			},
		},
		"ok, no alerts": {
			tenantID:    "foo",
			deviceID:    "bar",
			serviceName: "devicemonitor",
			resp: JSONResponseParams{
				OutputStatus: http.StatusOK,
			},
			callsUpsertAttributes: true,
			callsCheckAlerts:      true,
			alertsCount:           0,
			deviceAttributes: model.DeviceAttributes{
				{Name: model.AttrNameNumberOfAlerts, Value: 0, Scope: model.AttrScopeMonitor},
				{Name: model.AttrNameAlerts, Value: false, Scope: model.AttrScopeMonitor},
			},
		},
		"wrong service": {
			tenantID:    "foo",
			deviceID:    "bar",
			serviceName: "baz",
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("unsupported service"),
			},
		},
		"no device id": {
			tenantID:    "foo",
			serviceName: "baz",
			resp: JSONResponseParams{
				OutputStatus:     http.StatusBadRequest,
				OutputBodyObject: RestError("device id cannot be empty"),
			},
		},
		"ko, upsert attributes error": {
			tenantID:              "foo",
			deviceID:              "bar",
			serviceName:           "devicemonitor",
			callsCheckAlerts:      true,
			alertsCount:           0,
			callsUpsertAttributes: true,
			upsertAttributesErr:   errors.New("upsert attributes error"),
			resp: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
		"ko, check allerts error": {
			tenantID:         "foo",
			deviceID:         "bar",
			serviceName:      "devicemonitor",
			callsCheckAlerts: true,
			checkAlertsError: errors.New("check allerts error"),
			resp: JSONResponseParams{
				OutputStatus:     http.StatusInternalServerError,
				OutputBodyObject: RestError("internal error"),
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			var (
				inReq = test.MakeSimpleRequest("POST",
					"http://1.2.3.4/api/internal/v1/inventory/tenants/"+
						tc.tenantID+"/devices/"+tc.deviceID+"/reindex?service="+tc.serviceName,
					nil,
				)
			)

			inv := minventory.InventoryApp{}
			ctx := contextMatcher()

			if tc.callsUpsertAttributes {
				inv.On("UpsertAttributes",
					ctx,
					mock.AnythingOfType("model.DeviceID"),
					mock.MatchedBy(
						func(attrs model.DeviceAttributes) bool {
							if tc.deviceAttributes != nil {
								if !reflect.DeepEqual(tc.deviceAttributes, attrs) {
									assert.FailNow(t, "", "attributes not equal: %v \n%v\n", tc.deviceAttributes, attrs)
								}
							}
							return true
						},
					),
				).Return(tc.upsertAttributesErr)
			}
			if tc.callsCheckAlerts {
				inv.On("CheckAlerts",
					ctx,
					tc.deviceID,
				).Return(tc.alertsCount, tc.checkAlertsError)
			}

			apih := makeMockApiHandler(t, &inv)

			rest.ErrorFieldName = "error"

			runTestRequest(t, apih, inReq, tc.resp)

			inv.AssertExpectations(t)
		})
	}
}

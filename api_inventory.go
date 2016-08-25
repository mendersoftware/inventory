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
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/mendersoftware/inventory/config"
	"github.com/mendersoftware/inventory/log"
	"github.com/mendersoftware/inventory/requestlog"
	"github.com/mendersoftware/inventory/utils"
	"github.com/pkg/errors"
	"net/http"
)

const (
	uriDevices     = "/api/0.1.0/devices"
	uriDevice      = "/api/0.1.0/devices/:id"
	uriDeviceGroup = "/api/0.1.0/devices/:id/group"

	LogHttpCode = "http_code"
)

// model of device's group name response at /devices/:id/group endpoint
type InventoryApiGroup struct {
	Group string `json:"group"`
}

type InventoryFactory func(c config.Reader, l *log.Logger) (InventoryApp, error)

type InventoryHandlers struct {
	createInventory InventoryFactory
}

// return an ApiHandler for device admission app
func NewInventoryApiHandlers(invF InventoryFactory) ApiHandler {
	return &InventoryHandlers{
		invF,
	}
}

func (i *InventoryHandlers) GetApp() (rest.App, error) {
	routes := []*rest.Route{
		rest.Post(uriDevices, i.AddDeviceHandler),
	}

	routes = append(routes)

	app, err := rest.MakeRouter(
		// augment routes with OPTIONS handler
		AutogenOptionsRoutes(routes, AllowHeaderOptionsGenerator)...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create router")
	}

	return app, nil

}

func (i *InventoryHandlers) AddDeviceHandler(w rest.ResponseWriter, r *rest.Request) {
	l := requestlog.GetRequestLogger(r.Env)

	inv, err := i.createInventory(config.Config, l)
	if err != nil {
		restErrWithLogInternal(w, l, err)
		return
	}

	dev, err := parseDevice(r)
	if err != nil {
		restErrWithLog(w, l, err, http.StatusBadRequest)
		return
	}

	err = inv.AddDevice(dev)
	if err != nil {
		cause := errors.Cause(err)
		if cause != nil && cause == ErrDuplicatedDeviceId {
			restErrWithLogMsg(w, l, err, http.StatusBadRequest, "device with specified ID already exists")
			return
		}
		restErrWithLogInternal(w, l, err)
		return
	}

	l.F(log.Ctx{LogHttpCode: http.StatusCreated}).Info("ok")
	devurl := utils.BuildURL(r, uriDevice, map[string]string{
		":id": dev.ID.String(),
	})
	w.Header().Add("Location", devurl.String())
	w.WriteHeader(http.StatusCreated)
}

func parseDevice(r *rest.Request) (*Device, error) {
	dev := Device{}

	//decode body
	err := r.DecodeJsonPayload(&dev)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode request body")
	}

	//validate id
	if dev.ID == DeviceID("") {
		return nil, errors.New("'id' field required")
	}

	//validate attributes
	if len(dev.Attributes) > 0 {
		if err := validateDeviceAttributes(dev.Attributes); err != nil {
			return nil, err
		}
	}
	return &dev, nil
}

func validateDeviceAttributes(attributes DeviceAttributes) error {
	for _, attr := range attributes {
		if attr.Name == "" {
			return errors.New("attribute 'name' field required")
		}
		if attr.Value == nil {
			continue
		}
		switch attr.Value.(type) {
		case float64:
			break
		case string:
			break
		case []interface{}:
			arr := attr.Value.([]interface{})
			if !validateDeviceAttributeValueArray(arr) {
				return errors.New("invalid attribute value provided")
			}
		default:
			return errors.New("invalid attribute value provided")
		}
	}
	return nil
}

// device attributes value array can not have mixed types
func validateDeviceAttributeValueArray(arr []interface{}) bool {
	var firstValueString, firstValueFloat64 bool
	for i, v := range arr {
		_, isstring := v.(string)
		_, isfloat64 := v.(float64)
		if i == 0 {
			if isstring {
				firstValueString = true
			} else if isfloat64 {
				firstValueFloat64 = true
			} else {
				return false
			}
		} else if (firstValueString && !isstring) || (firstValueFloat64 && !isfloat64) {
			return false
		}
	}
	return true
}

// return selected http code + error message directly taken from error
// log error
func restErrWithLog(w rest.ResponseWriter, l *log.Logger, e error, code int) {
	restErrWithLogMsg(w, l, e, code, e.Error())
}

// return http 500, with an "internal error" message
// log full error
func restErrWithLogInternal(w rest.ResponseWriter, l *log.Logger, e error) {
	msg := "internal error"
	e = errors.Wrap(e, msg)
	restErrWithLogMsg(w, l, e, http.StatusInternalServerError, msg)
}

// return an error code with an overriden message (to avoid exposing the details)
// log full error
func restErrWithLogMsg(w rest.ResponseWriter, l *log.Logger, e error, code int, msg string) {
	rest.Error(w, msg, code)
	l.F(log.Ctx{LogHttpCode: code}).Error(errors.Wrap(e, msg).Error())
}

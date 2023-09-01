// Copyright 2023 Northern.tech AS
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
	"net/http"

	"github.com/ant0ine/go-json-rest/rest"
	"github.com/pkg/errors"

	"github.com/mendersoftware/go-lib-micro/log"

	api_http "github.com/mendersoftware/inventory/api/http"
	"github.com/mendersoftware/inventory/client/devicemonitor"
	"github.com/mendersoftware/inventory/client/workflows"
	"github.com/mendersoftware/inventory/config"
	inventory "github.com/mendersoftware/inventory/inv"
	"github.com/mendersoftware/inventory/store/mongo"
)

func SetupAPI(stacktype string) (*rest.Api, error) {
	api := rest.NewApi()
	if err := SetupMiddleware(api, stacktype); err != nil {
		return nil, errors.Wrap(err, "failed to setup middleware")
	}

	//this will override the framework's error resp to the desired one:
	// {"error": "msg"}
	// instead of:
	// {"Error": "msg"}
	rest.ErrorFieldName = "error"

	return api, nil
}

func RunServer(c config.Reader) error {

	l := log.New(log.Ctx{})

	db, err := mongo.NewDataStoreMongo(makeDataStoreConfig())
	if err != nil {
		return errors.Wrap(err, "database connection failed")
	}

	limitAttributes := c.GetInt(SettingLimitAttributes)
	limitTags := c.GetInt(SettingLimitTags)

	inv := inventory.NewInventory(db).WithLimits(limitAttributes, limitTags)

	devicemonitorAddr := c.GetString(SettingDevicemonitorAddr)
	if devicemonitorAddr != "" {
		c := devicemonitor.NewClient(devicemonitorAddr)
		inv = inv.WithDevicemonitor(c)
	}

	if inv, err = maybeWithInventory(inv, c); err != nil {
		return err
	}

	api, err := SetupAPI(c.GetString(SettingMiddleware))
	if err != nil {
		return errors.Wrap(err, "API setup failed")
	}

	invapi := api_http.NewInventoryApiHandlers(inv)
	apph, err := invapi.GetApp()
	if err != nil {
		return errors.Wrap(err, "inventory API handlers setup failed")
	}
	api.SetApp(apph)

	addr := c.GetString(SettingListen)
	l.Printf("listening on %s", addr)

	return http.ListenAndServe(addr, api.MakeHandler())
}

func maybeWithInventory(
	inv inventory.InventoryApp,
	c config.Reader,
) (inventory.InventoryApp, error) {
	if reporting := c.GetBool(SettingEnableReporting); reporting {
		orchestrator := c.GetString(SettingOrchestratorAddr)
		if orchestrator == "" {
			return inv, errors.New("reporting integration needs orchestrator address")
		}

		c := workflows.NewClient(orchestrator)
		inv = inv.WithReporting(c)
	}
	return inv, nil
}

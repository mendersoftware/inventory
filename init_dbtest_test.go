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

package main_test

import (
	"io/ioutil"
	"os"
	"testing"

	"gopkg.in/mgo.v2/dbtest"
	"log"
)

var db *dbtest.DBServer

// Overwrites test execution and allows for test database setup
func TestMain(m *testing.M) {
	log.Println("test")
	dbdir, _ := ioutil.TempDir("", "dbsetup-test")
	// os.Exit would ignore defers, workaround
	status := func() int {
		// Start test database server
		if !testing.Short() {
			db = &dbtest.DBServer{}
			db.SetPath(dbdir)
			// Tear down databaser server
			// Note:
			// if test panics, it will require manual database tier down
			// testing package executes tests in goroutines therefore
			// we can't catch panics issued in tests.
			defer os.RemoveAll(dbdir)
			defer db.Stop()
		}
		return m.Run()
	}()

	os.Exit(status)
}

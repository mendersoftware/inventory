#!/usr/bin/python
# Copyright 2016 Mender Software AS
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        https://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.
from common import internal_client, mongo, clean_db
import bravado
import pytest

class TestInternalApiTenantCreate:
    def test_create_ok(self, internal_client, clean_db):

        _, r = internal_client.create_tenant('foobar')
        assert r.status_code == 201

        assert 'inventory-foobar' in clean_db.database_names()
        assert 'migration_info' in clean_db['inventory-foobar'].collection_names()

    def test_create_twice(self, internal_client, clean_db):

        _, r = internal_client.create_tenant('foobar')
        assert r.status_code == 201

        # creating once more should not fail
        _, r = internal_client.create_tenant('foobar')
        assert r.status_code == 201


    def test_create_empty(self, internal_client):
        try:
            _, r = internal_client.create_tenant('')
        except bravado.exception.HTTPError as e:
            assert e.response.status_code == 400

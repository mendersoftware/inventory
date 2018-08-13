#!/usr/bin/python
# Copyright 2017 Northern.tech AS
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
import csv

import pytest

from pymongo import MongoClient

from client import CliClient, ManagementClient, InternalApiClient


@pytest.fixture(scope="session")
def mongo():
    return MongoClient('mender-mongo:27017')


def mongo_cleanup(mongo):
    dbs = mongo.database_names()
    dbs = [d for d in dbs if d not in ['local', 'admin']]
    for d in dbs:
        mongo.drop_database(d)


@pytest.yield_fixture(scope='function')
def clean_db(mongo):
    mongo_cleanup(mongo)
    yield mongo
    mongo_cleanup(mongo)


@pytest.fixture(scope="session")
def cli():
    return CliClient()


@pytest.fixture(scope="session")
def management_client():
    return ManagementClient()

@pytest.fixture(scope="session")
def internal_client():
    return InternalApiClient()


@pytest.fixture(scope="session")
def inventory_attributes(management_client):
    attributeList = []

    filename = pytest.config.getoption('--inventory-items')

    with open(filename) as inf:
        r = csv.reader(inf)
        for row in r:
            n, v, d = row[0], row[1], row[2] if len(row) == 3 else None
            # does it matter if you pass a field name = None?
            attr = management_client.inventoryAttribute(name=n,
                                                        value=v,
                                                        description=d)
            attributeList.append(attr)

    return attributeList

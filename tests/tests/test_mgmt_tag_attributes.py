# Copyright 2023 Northern.tech AS
#
#    Licensed under the Apache License, Version 2.0 (the "License");
#    you may not use this file except in compliance with the License.
#    You may obtain a copy of the License at
#
#        http://www.apache.org/licenses/LICENSE-2.0
#
#    Unless required by applicable law or agreed to in writing, software
#    distributed under the License is distributed on an "AS IS" BASIS,
#    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#    See the License for the specific language governing permissions and
#    limitations under the License.
from common import (
    inventory_attributes,
    management_client,
    internal_client,
    clean_db,
    mongo,
)

import bravado
import pytest
import requests

LIMIT_TAGS = 20


@pytest.mark.usefixtures("clean_db")
class TestTagAttributes:
    def test_set_tag_attributes_without_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        tags = {"n_1": {"name": "n_1", "value": "v_1", "description": "desc_1"}}
        tags_body = [tags["n_1"]]
        management_client.updateTagAttributes(did, tags_body)

        res = management_client.getDevice(did)
        tags_attributes = []
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                assert attr["name"] in tags
                tags_attributes.append(attr)
        assert len(tags_attributes) == len(tags)

    def test_update_tag_attributes_without_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        tags = {
            "n_1": {"name": "n_1", "value": "v_1", "description": "desc_1"},
            "n_2": {"name": "n_2", "value": "v_2", "description": "desc_2"},
        }
        tags_body = [tags["n_1"], tags["n_2"]]
        management_client.updateTagAttributes(did, tags_body)

        res = management_client.getDevice(did)
        tags_attributes = []
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                assert attr["name"] in tags
                tags_attributes.append(attr)
        assert len(tags_attributes) == len(tags_body)

    def test_replace_tag_attributes_without_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        tags = {"n_3": {"name": "n_3", "value": "v_3", "description": "desc_3"}}
        tags_body = [tags["n_3"]]
        management_client.setTagAttributes(did, tags_body)

        res = management_client.getDevice(did)
        tags_attributes = []
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                assert attr["name"] in tags
                tags_attributes.append(attr)
        assert len(tags_attributes) == len(tags_body)

    def test_update_tag_attributes_with_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        tags = [{"name": "n_4", "value": "v_4", "description": "desc_4"}]
        internal_client.create_device(did, inventory_attributes)
        management_client.updateTagAttributes(did, tags)

        res = requests.get(
            management_client.client.swagger_spec.api_url + "/devices/" + did
        )
        etag_one = res.headers["Etag"]
        tags += [{"name": "n_5", "value": 1.0}]
        management_client.updateTagAttributes(did, tags, eTag=etag_one)
        res = requests.get(
            management_client.client.swagger_spec.api_url + "/devices/" + did
        )
        etag_two = res.headers["Etag"]
        assert etag_one != etag_two

        res = management_client.getDevice(did)
        tags_attributes = []
        tag_names = [tag["name"] for tag in tags]
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                assert attr["name"] in tag_names
                tags_attributes.append(attr)
        assert len(tags_attributes) == len(tags)

    def test_replace_tag_attributes_with_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        tags = [{"name": "n_4", "value": "v_4", "description": "desc_4"}]
        internal_client.create_device(did, inventory_attributes)
        management_client.setTagAttributes(did, tags)

        res = requests.get(
            management_client.client.swagger_spec.api_url + "/devices/" + did
        )
        etag_one = res.headers["Etag"]
        tags += [{"name": "n_5", "value": 1.0}]
        management_client.setTagAttributes(did, tags, eTag=etag_one)
        res = requests.get(
            management_client.client.swagger_spec.api_url + "/devices/" + did
        )
        etag_two = res.headers["Etag"]
        assert etag_one != etag_two

        res = management_client.getDevice(did)
        tags_attributes = []
        tag_names = [tag["name"] for tag in tags]
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                assert attr["name"] in tag_names
                tags_attributes.append(attr)
        assert len(tags_attributes) == len(tags)

    def test_update_tag_attributes_with_wrong_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        tags = {"n_5": {"name": "n_5", "value": "v_5", "description": "desc_5"}}
        tags_body = [tags["n_5"]]
        internal_client.create_device(did, inventory_attributes)
        management_client.updateTagAttributes(did, tags_body)

        fake_etag = "241496e0-cbbb-4a83-90e9-70b4dd0e645a"
        try:
            management_client.updateTagAttributes(did, tags_body, eTag=fake_etag)
        except Exception as e:
            assert str(e) == "412 Precondition Failed"
        else:
            raise Exception("did not raise expected exception")

    def test_replace_tag_attributes_with_wrong_etag(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        tags = {"n_6": {"name": "n_6", "value": "v_6", "description": "desc_6"}}
        tags_body = [tags["n_6"]]
        internal_client.create_device(did, inventory_attributes)
        management_client.setTagAttributes(did, tags_body)

        fake_etag = "241496e0-cbbb-4a83-90e9-70b4dd0e645a"
        try:
            management_client.setTagAttributes(did, tags_body, eTag=fake_etag)
        except Exception as e:
            assert str(e) == "412 Precondition Failed"
        else:
            raise Exception("did not raise expected exception")

    def test_set_tags_truncates_because_of_limits(
        self, management_client, internal_client, inventory_attributes
    ):
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        tags_body = [
            {"name": "n_%03d" % i, "value": "v_%d" % i} for i in range(2 * LIMIT_TAGS)
        ]
        management_client.updateTagAttributes(did, tags_body)

        res = management_client.getDevice(did)
        tag_names = []
        for attr in res["attributes"]:
            if attr["scope"] == "tags":
                tag_names.append(attr["name"])
        assert len(tag_names) == LIMIT_TAGS
        assert tag_names == [t["name"] for t in tags_body[:LIMIT_TAGS]]

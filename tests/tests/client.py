from bravado.swagger_model import load_file
from bravado.client import SwaggerClient, RequestsClient
from urllib import parse as urlparse
from requests.utils import parse_header_links
import os
import requests
import pytest
import csv
from requests.packages.urllib3.exceptions import InsecureRequestWarning

requests.packages.urllib3.disable_warnings(InsecureRequestWarning)


class Client(object):
    config = {
        'also_return_response': True,
        'validate_responses': False,
        'validate_requests': False,
        'validate_swagger_spec': True,
        'use_models': True,
    }

    http_client = RequestsClient()
    http_client.session.verify = False

    client = SwaggerClient.from_spec(load_file('management_api.yml'), config=config, http_client=http_client)
    client.swagger_spec.api_url = "http://%s/api/%s" % (pytest.config.getoption("host"), pytest.config.getoption("api"))

    group = client.get_model("Group")
    deviceNew = client.get_model("DeviceNew")
    inventoryAttribute = client.get_model("Attribute")

    def getInventoryListFromFile(self, filename="inventory_items"):
        attributeList = []

        with open(filename) as inf:
            r = csv.reader(inf)
            for row in r:
                n, v, d = row[0], row[1], row[2] if len(row) == 3 else None
                # does it matter if you pass a field name = None?
                attributeList.append(self.inventoryAttribute(name=n, value=v, description=d))

        return attributeList

    def createDevice(self, attributes, deviceid=None, description="test device"):
        if not deviceid:
            deviceid = "".join([format(i, "02x") for i in os.urandom(128)])

        deviceNew = self.deviceNew(id=deviceid, description=description, attributes=attributes)
        r, h = self.client.devices.post_devices(device=deviceNew, Authorization="foo").result()
        return deviceid

    def deleteAllGroups(self):
        groups = self.client.groups.get_groups().result()[0]
        for g in groups:
            for d in self.getGroupDevices(g):
                self.deleteDeviceInGroup(g, d)

    def getAllDevices(self, page=1, sort=None, has_group=None):
        r, h = self.client.devices.get_devices(page=page, sort=sort, has_group=has_group, Authorization="foo").result()
        for i in parse_header_links(h.headers["link"]):
            if i["rel"] == "next":
                page = int(dict(urlparse.parse_qs(urlparse.urlsplit(i["url"]).query))["page"][0])
                return r + self.getAllDevices(page=page, sort=sort)
        else:
            return r

    def getAllGroups(self):
        r, _ = self.client.groups.get_groups().result()
        return r

    def getGroupDevices(self, group, expected_error=False):
        try:
            r = self.client.groups.get_groups_name_devices(name=group, Authorization=False).result()
        except Exception as e:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r[0]

    def deleteDeviceInGroup(self, group, device, expected_error=False):
        try:
            r = self.client.devices.delete_devices_id_group_name(id=device, name=group, Authorization=False).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r

    def addDeviceToGroup(self, group, device, expected_error=False):
        try:
            r = self.client.devices.put_devices_id_group(group=group, id=device, Authorization=False).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r

    def verifyInventory(self, inventoryItems, expected_data=None):
        if isinstance(expected_data, str):
            expectedInventoryItems = self.getInventoryListFromFile(expected_data)
        elif isinstance(expected_data, dict):
            expectedInventoryItems = []
            for k in expected_data.keys():
                expectedInventoryItems.append(self.inventoryAttribute(name=k, value=expected_data[k]))

        assert len(inventoryItems) == len(expected_data)

        for e in expected_data:
            if e not in inventoryItems:
                assert False, "Inventory data is incorrect"

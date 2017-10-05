from bravado.swagger_model import load_file
from bravado.client import SwaggerClient, RequestsClient
from urllib import parse as urlparse
from requests.utils import parse_header_links

import os
import requests
import pytest
import csv
import logging
import subprocess

from requests.packages.urllib3.exceptions import InsecureRequestWarning

requests.packages.urllib3.disable_warnings(InsecureRequestWarning)


class ManagementClient:
    config = {
        'also_return_response': True,
        'validate_responses': False,
        'validate_requests': False,
        'validate_swagger_spec': True,
        'use_models': True,
    }

    http_client = RequestsClient()
    http_client.session.verify = False

    client = SwaggerClient.from_spec(load_file(pytest.config.getoption("--management-spec")),
                                     config=config, http_client=http_client)
    client.swagger_spec.api_url = "http://%s/api/%s" % (pytest.config.getoption("host"),
                                                        pytest.config.getoption("api"))

    group = client.get_model("Group")
    deviceNew = client.get_model("DeviceNew")
    inventoryAttribute = client.get_model("Attribute")

    log = logging.getLogger('Client')

    def createDevice(self, attributes, deviceid=None, description="test device"):
        if not deviceid:
            deviceid = "".join([format(i, "02x") for i in os.urandom(32)])

        deviceNew = self.deviceNew(id=deviceid, description=description, attributes=attributes)
        r, h = self.client.devices.post_devices(device=deviceNew, Authorization="foo").result()
        return deviceid

    def deleteAllGroups(self):
        groups = self.client.groups.get_groups().result()[0]
        for g in groups:
            for d in self.getGroupDevices(g):
                self.deleteDeviceInGroup(g, d)

    def getAllDevices(self, page=1, sort=None, has_group=None):
        r, h = self.client.devices.get_devices(page=page, sort=sort, has_group=has_group,
                                               Authorization="foo").result()
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
            r = self.client.devices.delete_devices_id_group_name(id=device, name=group,
                                                                 Authorization=False).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r

    def addDeviceToGroup(self, group, device, expected_error=False):
        try:
            r = self.client.devices.put_devices_id_group(group=group, id=device,
                                                         Authorization=False).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r


class CliClient:
    cmd = '/testing/inventory'

    def migrate(self, tenant_id=None):
        args = [
            self.cmd,
            'migrate']

        if tenant_id:
            args += ['--tenant', tenant_id]

        subprocess.run(args, check=True)

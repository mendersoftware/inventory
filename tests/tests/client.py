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
        "also_return_response": True,
        "validate_responses": False,
        "validate_requests": False,
        "validate_swagger_spec": True,
        "use_models": True,
    }

    http_client = RequestsClient()
    http_client.session.verify = False

    client = SwaggerClient.from_spec(
        load_file(pytest.config.getoption("--management-spec")),
        config=config,
        http_client=http_client,
    )
    client.swagger_spec.api_url = "http://%s/api/%s" % (
        pytest.config.getoption("host"),
        pytest.config.getoption("api"),
    )

    group = client.get_model("Group")
    inventoryAttribute = client.get_model("Attribute")

    log = logging.getLogger("Client")

    def deleteAllGroups(self):
        groups = self.client.Management_API.List_Groups().result()[0]
        for g in groups:
            for d in self.getGroupDevices(g):
                self.deleteDeviceInGroup(g, d)

    def getAllDevices(self, page=1, sort=None, has_group=None, JWT="foo.bar.baz"):
        if not JWT.startswith("Bearer "):
            JWT = "Bearer " + JWT
        r, h = self.client.Management_API.List_Device_Inventories(
            page=page, sort=sort, has_group=has_group, Authorization=JWT
        ).result()
        for i in parse_header_links(h.headers["link"]):
            if i["rel"] == "next":
                page = int(
                    dict(urlparse.parse_qs(urlparse.urlsplit(i["url"]).query))["page"][
                        0
                    ]
                )
                return r + self.getAllDevices(page=page, sort=sort)
        else:
            return r

    def getDevice(self, device_id, Authorization="foo"):
        if not Authorization.startswith("Bearer "):
            Authorization = "Bearer " + Authorization
        r, _ = self.client.Management_API.Get_Device_Inventory(
            id=device_id, Authorization=Authorization
        ).result()
        return r

    def getAllGroups(self):
        r, _ = self.client.Management_API.List_Groups().result()
        return r

    def getGroupDevices(self, group, expected_error=False):
        try:
            r = self.client.Management_API.Get_Devices_in_Group(
                name=group, Authorization=False
            ).result()
        except Exception as e:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r[0]

    def deleteDeviceInGroup(self, group, device, expected_error=False):
        try:
            r = self.client.Management_API.Clear_Group(
                id=device, name=group, Authorization=False
            ).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r

    def addDeviceToGroup(self, group, device, expected_error=False, JWT="foo.bar.baz"):
        if not JWT.startswith("Bearer "):
            JWT = "Bearer " + JWT
        try:
            r = self.client.Management_API.Assign_Group(
                group=group, id=device, Authorization=JWT
            ).result()
        except Exception:
            if expected_error:
                return []
            else:
                pytest.fail()

        else:
            return r


class CliClient:
    cmd = "/testing/inventory"

    def migrate(self, tenant_id=None):
        args = [self.cmd, "migrate"]

        if tenant_id:
            args += ["--tenant", tenant_id]

        subprocess.run(args, check=True)


class ApiClient:
    config = {
        "also_return_response": True,
        "validate_responses": True,
        "validate_requests": False,
        "validate_swagger_spec": False,
        "use_models": True,
    }

    log = logging.getLogger("client.ApiClient")
    # override spec_option for internal vs management clients
    spec_option = "internal-spec"
    api_url = "http://%s/api/0.1.0/" % pytest.config.getoption("host")

    def make_api_url(self, path):
        return os.path.join(
            self.api_url, path if not path.startswith("/") else path[1:]
        )

    def setup_swagger(self):
        self.http_client = RequestsClient()
        self.http_client.session.verify = False

        spec = pytest.config.getoption(self.spec_option)
        self.client = SwaggerClient.from_spec(
            load_file(spec), config=self.config, http_client=self.http_client
        )
        self.client.swagger_spec.api_url = self.api_url

    def __init__(self):
        self.setup_swagger()


class InternalApiClient(ApiClient):
    log = logging.getLogger("client.InternalClient")
    spec_option = "internal_spec"
    api_url = "http://%s/api/internal/v1/inventory/" % pytest.config.getoption("host")

    def __init__(self):
        super().__init__()

    def verify(self, token, uri="/api/management/1.0/auth/verify", method="POST"):
        if not token.startswith("Bearer "):
            token = "Bearer " + token
        return self.client.auth.post_auth_verify(
            **{
                "Authorization": token,
                "X-Original-URI": uri,
                "X-Original-Method": method,
            }
        ).result()

    def DeviceNew(self, **kwargs):
        return self.client.get_model("DeviceNew")(**kwargs)

    def Attribute(self, **kwargs):
        return self.client.get_model("Attribute")(**kwargs)

    def create_tenant(self, tenant_id):
        return self.client.Internal_API.Create_Tenant(
            tenant={"tenant_id": tenant_id,}
        ).result()

    def create_device(self, device_id, attributes, description="test device"):
        device = self.DeviceNew(
            id=device_id, description=description, attributes=attributes
        )

        return self.client.Internal_API.Initialize_Device(device=device).result()

from client import Client
import os
import pytest
import random

class TestDeviceCreation(Client):

    def test_create_device_id_too_large(self):
        deviceid = "".join([ format(i, "02x") for i in os.urandom(508)])
        deviceNew = self.deviceNew(id=deviceid, attributes=self.getInventoryListFromFile())
        try:
            r, _ = self.client.devices.post_devices(device=deviceNew, Authorization="foo").result()
        except Exception as e:
            assert e.response.status_code == 500
        else:
            pytest.fail()

    def test_create_device_id_too_small(self):
        deviceNew = self.deviceNew(id="", attributes=self.getInventoryListFromFile())
        try:
            self.client.devices.post_devices(device=deviceNew, Authorization="foo").result()
        except Exception as e:
            assert "ID: non zero value required" in str(e)

    def test_create_device_and_get(self):
        deviceid = "".join([ format(i, "02x") for i in os.urandom(128)])

        deviceNew = self.deviceNew(id=deviceid, attributes=self.getInventoryListFromFile())
        self.client.devices.post_devices(device=deviceNew, Authorization="foo").result()
        r, _ = self.client.devices.get_devices_id(id=deviceid, Authorization="foo").result()
        self.verifyInventory(self.getInventoryListFromFile(), r.attributes)

    def test_get_client_nonexisting_id(self):
        deviceid = "test" + str(random.randint(0, 99999999))
        try:
            r, _ = self.client.devices.get_devices_id(id=deviceid, Authorization="foo").result()
        except Exception as e:
            assert e.response.status_code == 404
        else:
            pytest.fail()

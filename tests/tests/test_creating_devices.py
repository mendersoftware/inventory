from common import inventory_attributes, management_client, clean_db, mongo

import os
import pytest
import random


@pytest.mark.usefixtures("clean_db")
class TestDeviceCreation:

    def verify_inventory(self, inventory, expected):
        assert len(inventory) == len(expected)

        print("inventory:", inventory, "expected:", expected)
        for e in expected:
            if e not in inventory:
                assert False, "Inventory data is incorrect"


    def test_create_device_id_too_large(self, management_client, inventory_attributes):
        deviceid = "".join([ format(i, "02x") for i in os.urandom(508)])
        deviceNew = management_client.deviceNew(id=deviceid,
                                                attributes=inventory_attributes)
        try:
            r, _ = management_client.client.devices.post_devices(device=deviceNew,
                                                                 Authorization="foo").result()
        except Exception as e:
            assert e.response.status_code == 500
        else:
            pytest.fail()

    def test_create_device_id_too_small(self, management_client, inventory_attributes):
        deviceNew = management_client.deviceNew(id="",
                                                attributes=inventory_attributes)
        try:
            management_client.client.devices.post_devices(device=deviceNew,
                                                          Authorization="foo").result()
        except Exception as e:
            assert "ID: non zero value required" in str(e)

    def test_create_device_and_get(self, management_client, inventory_attributes):
        deviceid = "".join([ format(i, "02x") for i in os.urandom(128)])

        deviceNew = management_client.deviceNew(id=deviceid,
                                                attributes=inventory_attributes)
        management_client.client.devices.post_devices(device=deviceNew,
                                                      Authorization="foo").result()
        r, _ = management_client.client.devices.get_devices_id(id=deviceid,
                                                               Authorization="foo").result()
        self.verify_inventory(inventory_attributes,
                              [management_client.inventoryAttribute(name=attr.name,
                                                                    value=attr.value,
                                                                    description=attr.description) \
                               for attr in r.attributes])

    def test_get_client_nonexisting_id(self, management_client):
        deviceid = "test" + str(random.randint(0, 99999999))
        try:
            r, _ = management_client.client.devices.get_devices_id(id=deviceid,
                                                                   Authorization="foo").result()
        except Exception as e:
            assert e.response.status_code == 404
        else:
            pytest.fail()

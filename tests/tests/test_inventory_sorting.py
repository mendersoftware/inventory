from common import inventory_attributes, management_client, internal_client, clean_db, mongo

import pytest
import os


@pytest.mark.usefixtures("clean_db")
class TestInventorySorting:

    def test_inventory_sorting(self, management_client, internal_client, inventory_attributes):
        numbers = [100, 1000, 1, 999]

        for n in range(20):
            did = "".join([ format(i, "02x") for i in os.urandom(128)])
            internal_client.create_device(did, inventory_attributes)

        for n in numbers:
            it = list(inventory_attributes)
            it.append(internal_client.Attribute(name="number", value=n, scope="inventory"))

            did = "".join([ format(i, "02x") for i in os.urandom(128)])
            internal_client.create_device(did, it)

        t = []
        r = management_client.getAllDevices(sort="inventory-number:asc")
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers) == t

        t = []
        r, h = management_client.client.devices.get_devices(sort="inventory-number:desc",
                                                            Authorization="foo").result()
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers, reverse=True) == t

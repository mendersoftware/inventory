from common import inventory_attributes, management_client, clean_db, mongo

import pytest


@pytest.mark.usefixtures("clean_db")
class TestInventorySorting:

    def test_inventory_sorting(self, management_client, inventory_attributes):
        numbers = [100, 1000, 1, 999]

        for n in range(20):
            management_client.createDevice(attributes=inventory_attributes)

        for n in numbers:
            it = list(inventory_attributes)
            it.append(management_client.inventoryAttribute(name="number", value=n))
            management_client.createDevice(attributes=it)

        t = []
        r = management_client.getAllDevices(sort="number:asc")
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers) == t

        t = []
        r, h = management_client.client.devices.get_devices(sort="number:desc",
                                                            Authorization="foo").result()
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers, reverse=True) == t

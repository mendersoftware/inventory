from client import Client


class TestInventorySorting(Client):

    def test_inventory_sorting(self):
        numbers = [100, 1000, 1, 999]

        for n in range(20):
            self.createDevice(attributes=self.getInventoryListFromFile())

        for n in numbers:
            it = self.getInventoryListFromFile()
            it.append(self.inventoryAttribute(name="number", value=n))
            self.createDevice(attributes=it)

        t = []
        r = self.getAllDevices(sort="number:asc")
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers) == t

        t = []
        r, h = self.client.devices.get_devices(sort="number:desc", Authorization="foo").result()
        for deviceInventoryList in r:
            for i in deviceInventoryList.attributes:
                if i.name == "number":
                    t.append(i.value)

        assert sorted(numbers, reverse=True) == t

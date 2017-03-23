from client import Client
import requests

class TestInventorySearching(Client):

    def test_inventory_searching(self):
        extra_inventory_items = {"users_logged_in": 100, "open_connections": 1231, "open_ports": 523}

        for i in extra_inventory_items.keys():
            it = self.getInventoryListFromFile()
            it.append(self.inventoryAttribute(name=i, value=extra_inventory_items[i]))
            self.createDevice(attributes=it)

        r = requests.get(self.client.swagger_spec.api_url + "/devices", params=({"users_logged_in": 100}), verify=False)
        assert len(r.json()) == 1

        r = requests.get(self.client.swagger_spec.api_url + "/devices", params=({"open_connections": 1231}), verify=False)
        assert len(r.json()) == 1

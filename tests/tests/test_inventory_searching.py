from common import inventory_attributes, management_client, clean_db, mongo

import requests
import pytest


@pytest.mark.usefixtures("clean_db")
class TestInventorySearching:

    def test_inventory_searching(self, management_client, inventory_attributes):
        extra_inventory_items = {
            "users_logged_in": 100,
            "open_connections": 1231,
            "open_ports": 523,
        }

        for i in extra_inventory_items.keys():
            it = list(inventory_attributes)
            it.append(management_client.inventoryAttribute(name=i,
                                                           value=extra_inventory_items[i]))
            management_client.createDevice(attributes=it)

        r = requests.get(management_client.client.swagger_spec.api_url + "/devices",
                         params=({"users_logged_in": 100}),
                         verify=False)
        assert len(r.json()) == 1

        r = requests.get(management_client.client.swagger_spec.api_url + "/devices",
                         params=({"open_connections": 1231}),
                         verify=False)
        assert len(r.json()) == 1

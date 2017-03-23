from client import Client
import os
import pytest

class TestGroupRemoving(Client):

    def test_delete_device(self):
        d1 = self.createDevice(attributes=self.getInventoryListFromFile())
        g1 = "group-test-3"

        self.addDeviceToGroup(device=d1, group=self.group(group=g1))
        assert len(self.getGroupDevices(g1)) == 1

        self.deleteDeviceInGroup(device=d1, group=g1, expected_error=False)
        assert len(self.getGroupDevices(g1, expected_error=True)) == 0

    def test_delete_device_non_existent_1(self):
        """ Delete non-existent device from non-existent group """
        g1 = "group-test-3-non-existent"
        self.deleteDeviceInGroup(device="404 device", group=g1, expected_error=True)

    def test_delete_device_non_existent_2(self):
        """ Delete existent device from non-existent group """
        d1 = self.createDevice(attributes=self.getInventoryListFromFile())
        self.deleteDeviceInGroup(device=d1, group="404 group", expected_error=True)

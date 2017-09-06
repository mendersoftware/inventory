from client import Client
import os
import pytest


class TestGroupCreation(Client):

    def test_get_groups_is_empty(self):
        assert len(self.client.groups.get_groups().result()[0]) == 0

    def test_moving_device_group_1(self):
        """
        Create 1 device and move it amung 2 different groups
        """
        did = self.createDevice(attributes=self.getInventoryListFromFile())
        group = self.group(group="groupA")
        self.client.devices.put_devices_id_group(group=group, id=did, Authorization="foo").result()
        print( self.getGroupDevices("groupA"))
        assert len(self.getGroupDevices("groupA")) == 1

        group = self.group(group="groupB")
        self.client.devices.put_devices_id_group(group=group, id=did, Authorization="foo").result()

        assert len(self.getGroupDevices("groupA", expected_error=True)) == 0
        assert len(self.getGroupDevices("groupB")) == 1

    def test_moving_devices_1(self):
        """
        Create 2 devices and move them amoung 2 different groups
        """
        d1 = self.createDevice(attributes=self.getInventoryListFromFile())
        d2 = self.createDevice(attributes=self.getInventoryListFromFile())

        group = self.group(group="group-test-1")
        self.addDeviceToGroup(group=group, device=d1)
        self.addDeviceToGroup(group=group, device=d2)
        assert len(self.getGroupDevices("group-test-1")) == 2

        group = self.group(group="group-test-2")
        self.addDeviceToGroup(group=group, device=d2)
        assert len(self.getGroupDevices("group-test-1")) == 1
        assert len(self.getGroupDevices("group-test-2")) == 1

        self.addDeviceToGroup(group=group, device=d1)
        assert len(self.getGroupDevices("group-test-1", expected_error=True)) == 0
        assert len(self.getGroupDevices("group-test-2")) == 2

        group = self.group(group="group-test-1")
        self.addDeviceToGroup(group=group, device=d1)
        self.addDeviceToGroup(group=group, device=d2)
        assert len(self.getGroupDevices("group-test-1")) == 2
        assert len(self.getGroupDevices("group-test-2", expected_error=True)) == 0

    def test_get_groups(self):
        """
        Create 100 groups, each containing a single device
        """
        self.deleteAllGroups()

        for i in range(100):
            group = self.group(group="group" + str(i))
            self.client.devices.put_devices_id_group(group=group,
                                                     Authorization="foo",
                                                     id=self.createDevice(attributes=self.getInventoryListFromFile())).result()

        assert len(self.client.groups.get_groups().result()[0]) == 100
        self.deleteAllGroups()

    def test_get_groups_3(self):
        """
        Create 1 device, and move through 100 different groups
        """
        self.deleteAllGroups()

        device = self.createDevice(attributes=self.getInventoryListFromFile())
        for i in range(100):
            group = self.group(group="group" + str(i))
            self.client.devices.put_devices_id_group(group=group, id=device, Authorization="foo").result()

        assert len(self.client.groups.get_groups().result()[0]) == 1
        self.deleteAllGroups()

    def test_has_group(self):
        """
            Verify has_group functionality
        """
        self.deleteAllGroups()
        did = self.createDevice(attributes=self.getInventoryListFromFile())
        assert len(self.getAllGroups()) == 0
        assert len(self.getAllDevices(has_group=True)) == 0

        group = self.group(group="has_group_test_1")
        self.addDeviceToGroup(group=group, device=did)
        assert len(self.getAllDevices(has_group=True)) == 1

        self.deleteDeviceInGroup(group="has_group_test_1", device=did)
        assert len(self.getAllDevices(has_group=True)) == 0

    def test_generic_groups_1(self):
        self.deleteAllGroups()
        total_groups = 10
        items_per_group = 2
        devices_in_groups = {}

        for i in range(total_groups):
            group = self.group(group="group" + str(i))
            for j in range(items_per_group):
                device = self.createDevice(attributes=self.getInventoryListFromFile())
                devices_in_groups.setdefault(str(i), []).append(device)
                self.client.devices.put_devices_id_group(group=group, id=device, Authorization="foo").result()

        assert len(self.getAllGroups()) == 10

        groups = self.getAllGroups()
        for idx, g in enumerate(groups):
            assert sorted(self.getGroupDevices(g)) == sorted(devices_in_groups[str(idx)])

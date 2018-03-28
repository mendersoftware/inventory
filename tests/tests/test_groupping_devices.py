from common import inventory_attributes, management_client, internal_client, clean_db, mongo

import os
import pytest



@pytest.mark.usefixtures("clean_db")
class TestGroupCreation:

    def test_get_groups_is_empty(self, management_client):
        assert len(management_client.client.groups.get_groups().result()[0]) == 0

    def test_moving_device_group_1(self, management_client, internal_client, inventory_attributes):
        """
        Create 1 device and move it amung 2 different groups
        """
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        group = management_client.group(group="groupA")
        management_client.client.devices.put_devices_id_group(group=group,
                                                              id=did, Authorization="foo").result()
        group_a_devs = management_client.getGroupDevices("groupA")
        print(group_a_devs)
        assert len(group_a_devs) == 1

        group = management_client.group(group="groupB")
        management_client.client.devices.put_devices_id_group(group=group,
                                                              id=did, Authorization="foo").result()

        assert len(management_client.getGroupDevices("groupA", expected_error=True)) == 0
        assert len(management_client.getGroupDevices("groupB")) == 1

    def test_moving_devices_1(self, management_client, internal_client, inventory_attributes):
        """
        Create 2 devices and move them amoung 2 different groups
        """
        did1 = "device-id-1"
        did2 = "device-id-2"
        internal_client.create_device(did1, inventory_attributes)
        internal_client.create_device(did2, inventory_attributes)

        group = management_client.group(group="group-test-1")
        management_client.addDeviceToGroup(group=group, device=did1)
        management_client.addDeviceToGroup(group=group, device=did2)
        assert len(management_client.getGroupDevices("group-test-1")) == 2

        group = management_client.group(group="group-test-2")
        management_client.addDeviceToGroup(group=group, device=did2)
        assert len(management_client.getGroupDevices("group-test-1")) == 1
        assert len(management_client.getGroupDevices("group-test-2")) == 1

        management_client.addDeviceToGroup(group=group, device=did1)
        assert len(management_client.getGroupDevices("group-test-1",
                                                     expected_error=True)) == 0
        assert len(management_client.getGroupDevices("group-test-2")) == 2

        group = management_client.group(group="group-test-1")
        management_client.addDeviceToGroup(group=group, device=did1)
        management_client.addDeviceToGroup(group=group, device=did2)
        assert len(management_client.getGroupDevices("group-test-1")) == 2
        assert len(management_client.getGroupDevices("group-test-2",
                                                     expected_error=True)) == 0

    def test_get_groups(self, management_client, internal_client, inventory_attributes):
        for i in range(10):
            group = management_client.group(group="group" + str(i))
            did = "".join([ format(i, "02x") for i in os.urandom(128)])
            internal_client.create_device(did, inventory_attributes)
            management_client.client.devices.put_devices_id_group(group=group,
                                                                  Authorization="foo",
                                                                  id=did).result()

        assert len(management_client.client.groups.get_groups().result()[0]) == 10

    def test_get_groups_3(self, management_client, internal_client, inventory_attributes):
        """
        Create 1 device, and move through 100 different groups
        """

        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        for i in range(10):
            group = management_client.group(group="group" + str(i))
            management_client.client.devices.put_devices_id_group(group=group,
                                                                  id=did,
                                                                  Authorization="foo").result()

        assert len(management_client.client.groups.get_groups().result()[0]) == 1

    def test_has_group(self, management_client, internal_client, inventory_attributes):
        """
            Verify has_group functionality
        """
        did = "some-device-id"
        internal_client.create_device(did, inventory_attributes)
        assert len(management_client.getAllGroups()) == 0
        assert len(management_client.getAllDevices(has_group=True)) == 0

        group = management_client.group(group="has_group_test_1")
        management_client.addDeviceToGroup(group=group, device=did)
        assert len(management_client.getAllDevices(has_group=True)) == 1

        management_client.deleteDeviceInGroup(group="has_group_test_1", device=did)
        assert len(management_client.getAllDevices(has_group=True)) == 0

    def test_generic_groups_1(self, management_client, internal_client, inventory_attributes):
        total_groups = 10
        items_per_group = 2
        devices_in_groups = {}

        for i in range(total_groups):
            group = management_client.group(group="group" + str(i))
            for j in range(items_per_group):
                device = "".join([ format(i, "02x") for i in os.urandom(128)])
                internal_client.create_device(device, inventory_attributes)
                devices_in_groups.setdefault(str(i), []).append(device)
                management_client.client.devices.put_devices_id_group(group=group,
                                                                      id=device,
                                                                      Authorization="foo").result()

        assert len(management_client.getAllGroups()) == 10

        groups = management_client.getAllGroups()
        for idx, g in enumerate(groups):
            assert sorted(management_client.getGroupDevices(g)) == sorted(devices_in_groups[str(idx)])

import requests
import pymongo

from common import mongo, clean_db
from generate import rstr, rstrarr, rnum, rnumarr, rstrgen, dev

def test_regex_single_string_attr(clean_db):
    '''
        simplest possible test
        couple devs with just a single field conforming to a pattern
    '''
    devs = []

    # 9 devs matching A1
    for _ in range(5):
        devs.append(
            dev({
            'sn': '{}_A1_{}'.format(rstr(4), rstr(4)),
        }))
    for _ in range(3):
        devs.append(
            dev({
            'sn': 'A1_{}'.format(rstr(10)),
        }))
    for _ in range(1):
        devs.append(
            dev({
            'sn': '{}_A1'.format(rstr(10)),
        }))

    # 3 devs matching B1
    for _ in range(3):
        devs.append(
            dev({
            'sn': 'B1_{}'.format(rstr(10)),
        }))

    # 3 unrelated devs
    for _ in range(3):
        devs.append(
            dev({
            'sn': '{}'.format(rstr(10)),
        }))

    clean_db.inventory.devices.insert_many(devs)

    # verify
    r = api_search({'sn': '~A1'})
    assert len(r.json()) == 9

    r = api_search({'sn': 'regex:A1'})
    assert len(r.json()) == 9

    r = api_search({'sn': '~B1'})
    assert len(r.json()) == 3

def test_regex_many_string_attrs(clean_db):
    '''
        typical scenario
        couple devs with >1 attributes, some conforming to a pattern
    '''
    devs = []

    for _ in range(2):
        devs.append(
            dev({
            'vendor-id': 'V1_{}'.format(rstr(4)),
            'sn': 'sn-{}_A1_{}'.format(rstr(4), rstr(4)),
        }))

    for _ in range(3):
        devs.append(
            dev({
            'vendor-id': 'V2_{}'.format(rstr(4)),
            'sn': 'sn-{}_A2_{}'.format(rstr(4), rstr(4)),
        }))

    clean_db.inventory.devices.insert_many(devs)

    # verify
    r = api_search({'vendor-id': '~V1'})
    assert len(r.json()) == 2

    r = api_search({'vendor-id': '~V2'})
    assert len(r.json()) == 3

    r = api_search({'sn': '~A1'})
    assert len(r.json()) == 2

    r = api_search({'sn': '~A2'})
    assert len(r.json()) == 3

    r = api_search({'vendor-id': '~A2'})
    assert len(r.json()) == 0

def test_regex_strings_and_string_arrays(clean_db):
    '''
        some regexes match in simple attributes,
        some in sub-array values
    '''
    devs = []

    for _ in range(2):
        devs.append(
            dev({
            'vendor-id': 'V1_{}'.format(rstr(4)),
            'foos': rstrarr(3, 'V1_{}', rstrgen(3)),
            'bars': ['common', 'B1', 'cc']
        }))

    for _ in range(2):
        devs.append(
            dev({
            'vendor-id': 'V2_{}'.format(rstr(4)),
            'foos': rstrarr(3, 'V2_{}', rstrgen(3)),
            'bars': ['common', 'cc', 'B2']
        }))

    clean_db.inventory.devices.insert_many(devs)

    # verify
    r = api_search({'foos': '~V1'})
    assert len(r.json()) == 2

    r = api_search({'foos': '~V2'})
    assert len(r.json()) == 2

    r = api_search({'bars': '~B1'})
    assert len(r.json()) == 2

    r = api_search({'bars': '~B2'})
    assert len(r.json()) == 2

    r = api_search({'bars': '~common'})
    assert len(r.json()) == 4

def test_regex_complex(clean_db):
    '''
        devices with simple string attributes, 
        some actual regexes, not just pre/post/infix
    '''
    devs = []

    devs.append(
        dev({
            'mac': 'de:ad:be:ef:00:00',
            'sn': 'vendor-00001-12345667',
            'ip': 'invalid',
    }))

    devs.append(
        dev({
            'mac': 'de:ad:be:ef:00:01',
            'sn': 'vendor-00001-12',
            'ip': '192.0.0.1',
    }))

    devs.append(
        dev({
            'mac': 'de:ad:be:ef:00:02',
            'sn': 'vendor-00001-12aaaa',
            'ip': '192.0.0.2',
    }))

    devs.append(
        dev({
            'mac': 'not really a mac',
            'sn': 'vendor-00001-12aaaa',
            'ip': '192.0.0.3',
    }))

    clean_db.inventory.devices.insert_many(devs)

    # verify
    r = api_search({'mac': '~^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$'})
    assert len(r.json()) == 3

    r = api_search({'ip': '~^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$'})
    assert len(r.json()) == 3

    r = api_search({'mac': 'de:ad:be:ef:00:01'})
    assert len(r.json()) == 1

def api_search(opts={}):
    return requests.get("http://mender-inventory:8080/api/0.1.0/devices", opts)

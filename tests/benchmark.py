#!/usr/bin/env python3

import argparse
import random
import subprocess
import os

from pymongo import MongoClient

from generate import rstr, rstrarr, rnum, rnumarr, rstrgen, dev

URL='http://localhost:8080/api/0.1.0/devices?'
mongo = None

def run(bench, c='100', n='10000'):
    query = bench()
    url = with_qs(URL, query) 
    subprocess.run(
        [os.getenv('GOPATH')+'/bin/bombardier',
        '-c', c,
        '-n', n,
        url])

def with_qs(url, qs):
    for q in qs:
        url += '{}={}&'.format(q, qs[q])
    return url

# bench funcs return test query dict
def _do_simple_attr(ndevs, nmatching, nattrs, where, index=False):
    ''' generate ndevs devices, of which there will be nmatches (pattern 'A1', where = [prefix|infix]), 
        each dev has a total of nattrs attributes
    '''
    print('benchmark=simple attr, ndevs={}, nmatching={}, nattrs={}, where={}'.format(ndevs, nmatching, nattrs, where))
    global mongo

    devs = []
    for _ in range(ndevs):
        if nmatching > 0:
            if where == 'prefix':
                d = dev({'sn': 'A1{}'.format(rstr(8))})
            elif where == 'infix':
                d = dev({'sn': '{}A1{}'.format(rstr(4), rstr(4))})
            nmatching -= 1
        else:
            d = dev({'sn': '{}'.format(rstr(10))})

        for i in range(nattrs-1):
            d['foo{}'.format(i)] = '{}'.format(rstr(10))

        devs.append(d)

    mongo.inventory.devices.insert_many(devs)
    if index:
        mongo.inventory.devices.create_index('attributes.sn.value')

    assert mongo.inventory.devices.find({'devices.sn.value':{'$regex':'A1'}}).count() == nmatching 

    if where == 'prefix':
        return {'sn': '~^A1'}
    else:
        return {'sn': '~A1'}

def _do_simple_attr_mac(ndevs, nmatching, nattrs, index=False):
    ''' like _do_simple_attr, but the matching attribute is a mac,
        and the query is a not so trivial regex
    '''
    print('benchmark=simple attrmac, ndevs={}, nmatching={}, nattrs={}'.format(ndevs, nmatching, nattrs))

    global mongo

    devs = []
    for _ in range(ndevs):
        if nmatching > 0:
            d = dev({'mac': "02:00:00:%02x:%02x:%02x" % (random.randint(0, 255), random.randint(0, 255), random.randint(0, 255))})
            nmatching -= 1
        else:
            d = dev({'foo': '{}'.format(rstr(10))})

        for i in range(nattrs-1):
            d['foo{}'.format(i)] = '{}'.format(rstr(10))

        devs.append(d)

    mongo.inventory.devices.insert_many(devs)
    if index:
        mongo.inventory.devices.create_index('attributes.mac.value')

    assert mongo.inventory.devices.find({'devices.mac.value':{'$regex':'^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$'}}).count() == nmatching
    return {'mac': '~^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$'}

benchmarks = {
    '1_attr_10_devs_prefix': lambda : _do_simple_attr(10, 10, 1, 'prefix'),
    '1_attr_100_devs_prefix': lambda : _do_simple_attr(100, 10, 1, 'prefix'),
    '1_attr_500_devs_prefix': lambda : _do_simple_attr(500, 10, 1, 'prefix'),
    '1_attr_1000_devs_prefix': lambda : _do_simple_attr(1000, 10, 1, 'prefix'),
    '1_attr_5000_devs_prefix': lambda : _do_simple_attr(5000, 10, 1, 'prefix'),
    '5_attr_10_devs_prefix': lambda : _do_simple_attr(10, 10, 5, 'prefix'),
    '5_attr_100_devs_prefix': lambda : _do_simple_attr(100, 10, 5, 'prefix'),
    '5_attr_500_devs_prefix': lambda : _do_simple_attr(500, 10, 5, 'prefix'),
    '5_attr_1000_devs_prefix': lambda : _do_simple_attr(1000, 10, 5, 'prefix'),
    '5_attr_5000_devs_prefix': lambda : _do_simple_attr(5000, 10, 5, 'prefix'),
    '5_attr_50000_devs_prefix': lambda : _do_simple_attr(50000, 10, 5, 'prefix'),
    '10_attr_10_devs_prefix': lambda : _do_simple_attr(10, 10, 10, 'prefix'),
    '10_attr_100_devs_prefix': lambda : _do_simple_attr(100, 10, 10, 'prefix'),
    '10_attr_500_devs_prefix': lambda : _do_simple_attr(500, 10, 10, 'prefix'),
    '10_attr_1000_devs_prefix': lambda : _do_simple_attr(1000, 10, 10, 'prefix'),
    '10_attr_5000_devs_prefix': lambda : _do_simple_attr(5000, 10, 10, 'prefix'),
    '10_attr_10000_devs_prefix': lambda : _do_simple_attr(10000, 10, 10, 'prefix'),
    '10_attr_20000_devs_prefix': lambda : _do_simple_attr(20000, 10, 10, 'prefix'),
    '10_attr_50000_devs_prefix': lambda : _do_simple_attr(50000, 10, 10, 'prefix'),
    '10_attr_5000_devs_prefix_index': lambda : _do_simple_attr(5000, 10, 10, 'prefix', index=True),
    '10_attr_10000_devs_prefix_index': lambda : _do_simple_attr(10000, 10, 10, 'prefix', index=True),
    '10_attr_20000_devs_prefix_index': lambda : _do_simple_attr(20000, 10, 10, 'prefix', index=True),
    '10_attr_50000_devs_prefix_index': lambda : _do_simple_attr(50000, 10, 10, 'prefix', index=True),
    '10_attr_100000_devs_prefix_index': lambda : _do_simple_attr(100000, 10, 10, 'prefix', index=True),
    '10_attr_200000_devs_prefix_index': lambda : _do_simple_attr(200000, 10, 10, 'prefix', index=True),
    '10_attr_1000000_devs_prefix_index': lambda : _do_simple_attr(1000000, 10, 10, 'prefix', index=True),
    '1_attr_10_devs_infix': lambda : _do_simple_attr(10, 10, 1, 'infix'),
    '1_attr_100_devs_infix': lambda : _do_simple_attr(100, 10, 1, 'infix'),
    '1_attr_500_devs_infix': lambda : _do_simple_attr(500, 10, 1, 'infix'),
    '1_attr_1000_devs_infix': lambda : _do_simple_attr(1000, 10, 1, 'infix'),
    '1_attr_5000_devs_infix': lambda : _do_simple_attr(5000, 10, 1, 'infix'),
    '5_attr_10_devs_infix': lambda : _do_simple_attr(10, 10, 5, 'infix'),
    '5_attr_100_devs_infix': lambda : _do_simple_attr(100, 10, 5, 'infix'),
    '5_attr_500_devs_infix': lambda : _do_simple_attr(500, 10, 5, 'infix'),
    '5_attr_1000_devs_infix': lambda : _do_simple_attr(1000, 10, 5, 'infix'),
    '5_attr_5000_devs_infix': lambda : _do_simple_attr(5000, 10, 5, 'infix'),
    '10_attr_10_devs_infix': lambda : _do_simple_attr(10, 10, 10, 'infix'),
    '10_attr_100_devs_infix': lambda : _do_simple_attr(100, 10, 10, 'infix'),
    '10_attr_500_devs_infix': lambda : _do_simple_attr(500, 10, 10, 'infix'),
    '10_attr_1000_devs_infix': lambda : _do_simple_attr(1000, 10, 10, 'infix'),
    '10_attr_5000_devs_infix': lambda : _do_simple_attr(5000, 10, 10, 'infix'),
    '10_attr_10000_devs_infix': lambda : _do_simple_attr(10000, 10, 10, 'infix'),
    '10_attr_20000_devs_infix': lambda : _do_simple_attr(20000, 10, 10, 'infix'),
    '10_attr_50000_devs_infix': lambda : _do_simple_attr(50000, 10, 10, 'infix'),
    '10_attr_100000_devs_infix': lambda : _do_simple_attr(100000, 10, 10, 'infix'),
    '1_attr_10_devs_mac': lambda : _do_simple_attr_mac(10, 10, 1),
    '1_attr_100_devs_mac': lambda : _do_simple_attr_mac(100, 10, 1),
    '1_attr_500_devs_mac': lambda : _do_simple_attr_mac(500, 10, 1),
    '1_attr_1000_devs_mac': lambda : _do_simple_attr_mac(1000, 10, 1),
    '1_attr_5000_devs_mac': lambda : _do_simple_attr_mac(5000, 10, 1),
    '5_attr_10_devs_mac': lambda : _do_simple_attr_mac(10, 10, 5),
    '5_attr_100_devs_mac': lambda : _do_simple_attr_mac(100, 10, 5),
    '5_attr_500_devs_mac': lambda : _do_simple_attr_mac(500, 10, 5),
    '5_attr_1000_devs_mac': lambda : _do_simple_attr_mac(1000, 10, 5),
    '5_attr_5000_devs_mac': lambda : _do_simple_attr_mac(5000, 10, 5),
    '10_attr_10_devs_mac': lambda : _do_simple_attr_mac(10, 10, 10),
    '10_attr_100_devs_mac': lambda : _do_simple_attr_mac(100, 10, 10),
    '10_attr_500_devs_mac': lambda : _do_simple_attr_mac(500, 10, 10),
    '10_attr_1000_devs_mac': lambda : _do_simple_attr_mac(1000, 10, 10),
    '10_attr_5000_devs_mac': lambda : _do_simple_attr_mac(5000, 10, 10),
    '10_attr_10000_devs_mac': lambda : _do_simple_attr_mac(10000, 10, 10),
    '10_attr_20000_devs_mac': lambda : _do_simple_attr_mac(20000, 10, 10),
    '10_attr_100000_devs_mac': lambda : _do_simple_attr_mac(100000, 10, 10),
    '10_attr_200000_devs_mac': lambda : _do_simple_attr_mac(100000, 10, 10),
    '10_attr_5000_devs_mac_index': lambda : _do_simple_attr_mac(5000, 10, 10, index=True),
    '10_attr_10000_devs_mac_index': lambda : _do_simple_attr_mac(10000, 10, 10, index=True),
    '10_attr_20000_devs_mac_index': lambda : _do_simple_attr_mac(20000, 10, 10, index=True),
    '10_attr_50000_devs_mac_index': lambda : _do_simple_attr_mac(50000, 10, 10, index=True),
    '10_attr_100000_devs_mac_index': lambda : _do_simple_attr_mac(100000, 10, 10, index=True),
    '10_attr_200000_devs_mac_index': lambda : _do_simple_attr_mac(200000, 10, 10, index=True),
}

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='run an inventory search benchmark on a selected benchmark func.')
    parser.add_argument('benchmark', metavar='benchmark', nargs='+', help='name of the benchmark function (starts with bench_)')
    parser.add_argument('-c', dest='conns', default='100', help='number of concurrent connections, default=100')

    args = parser.parse_args()

    mongo = MongoClient()
    mongo.drop_database('inventory')

    print('running at {} conns'.format(args.conns))

    bfun = benchmarks[args.benchmark[0]]
    if bfun is not None:
        run(bfun, c=args.conns)
    else:
        for f in benchmarks:
            run(benchmarks[f], c=args.conns)

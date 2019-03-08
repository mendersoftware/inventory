import random 
import string

# a bunch of generator funcs - random data according to patterns
def rnum(min, max):
    return random.uniform(min, max)

def rnumarr(min, max, len):
    return [random.uniform(min, max) for _ in range(len)]

def rstr(n):
    return ''.join(random.choice(string.ascii_lowercase + string.digits) for _ in range(n))

def rstrgen(n):
    return lambda: rstr(n)

def rstrarr(len, pattern, *gens):
    ret = []
    for _ in range(len):
        generated = [g() for g in gens]
        ret.append(pattern.format(*generated))
    return ret

# create an inventory-compatible device struct from an attribute dict
def dev(attrs):
    dev = {
        'id': rstr(10),
        'attributes':{}
    }
    for k in attrs:
        dev['attributes'][k] = {'name': k, 'value': attrs[k], 'description': 'bogus filler description'}
    return dev

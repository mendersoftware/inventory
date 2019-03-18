This documents a simple microbenchmark created for the regex search functionality. 
The aim is to get a general feel of how far regexes can take us performance-wise.

The regex search functionality is POC'ed in the service code in this branch.

Benchmarking is done indirectly, via http benchmarking of the search endpoint (instead of db-level benchmarking). This is because of 
some awkwardness of mongo perf testing tools (need js scripting, and/or old python, and/or done via mongo shell...) - 
no time to figure it out for this POC. The tool of choice is https://github.com/codesenberg/bombardier.

Benchmark collected metrics:
- latency(ms)
- req/s

Benchmark variables:
- number of devices (10 to 100k's in select cases)
- number of attributes per device (1, 5, 10)
- regex query:
    - prefix (`^A1`)
        - will find only anchored values like `A1_foo`
        - the simplest query
    - infix (`A1`)
        - will find `A1` anywhere, more demanding (full scan)
    - mac
        - anchored MAC regex (`^...$`)
        - = 'an actual regex', pretty demanding (good amt of matching + a terminating anchor)
    - all queries on a single field
- number of concurrent connections
    - 10 - 100, in select cases up to 1000

For each request, sampling is done over 10k requests total.

# Disclaimer
The benchmarks are of course naive - local dev laptop, single tenant db and some simple patterns.
There are multiple moving parts that can affect production:
- actual traffic
- data distribution (num devs/num attrs)
- actual queries (we're giving users a potential DoS weapon)
- perf characteristics of our mongo provider
- stuff like page/cache thrashing (e.g. between tenants - how is memory used between dbs).

IMO this can't be guessed. If we go with this solution, the only way is to monitor inventory
and its db for long queries, and go from there (commit to a different solution altogether if necessary).

# How to run
Note that benchmarks run on *your* machine, not in docker.

Requires:
1. go 1.11 + bombardier: 
`go get -u github.com/codesenberg/bombardier`

2. `pip3 install pymongo`

3. mongodb running (defaults)

4. running inventory:
`INVENTORY_MONGO=localhost:27017 inventory server --automigrate` 

To run a benchmark:
`cd tests && ./benchmark.py <name_of_benchmark> -c <num_connections>`

Benchmarks are predefined in the 'benchmarks' dict. 

# Results

Full results are available as a calc sheet at https://docs.google.com/spreadsheets/d/1ZuAWO3ymPx994vFIKmAJE7UUTj_x1unBYszgy879tZI/edit?usp=sharing.

There were 2 runs of the benchmark:
- without indexes
    - tested because of some limitiations of indexes 
    - good to know the absolute worst case
- with indexes
    - after some discussions re: bad performance of no indexes

IMPORTANT: the first sets of data were collected with a slightly buggy benchmark - sheet simple-noindex.
Some runs were repeated after the fix - sheet FIXED-simple-noindex.
I'm leaving the buggy results in, because they have an almost perfect x2 relation to the 'fixed' values.
For the sake of time I'm not repeating all those runs, so just use the initial vals as a reference.

## Discussion
Let's assume some limits/rules of thumb we're aiming at; these are based on different numbers floating
around the net. It's a very open question as to what the numbers should be:

- latency: 100-200ms
    - 100ms quoted as 'max comfortable' latency for an actual human user
    - let's make 200ms a stretch value
    - twitter(2008): 200ms

- req/s: max low hundreds?
    - stackexachange(2013): 300
    - twitter(2008): 600

As for number of concurrently supported connections, this is very case by case. 
In general, X users perform N daily visits of length L - some hits will be to the search EP, 
and some will be concurrent. There are tricks to estimate this based on known user scenarios, 
but we decided to settle for a ~10 guestimate.

### Run 1 - No indexing

We can go up to as many as 50k (10 attrs) devices with comfortable latencies at max 10 concurrent connections.
Near 50 concurrent connections the latencies break down.

### Run 2 - with indexing
We decided the indexing is worth pursuing still, even despite the limitations.

Indexes work for anchored queries only so prefix and mac queries were tested

We went straight for a larger number of devices - from 5000 to 200000, i.e. pick up where we left off
with unindexed queries.

The results at at sheet #2 of the calc linked above, and are promising.

As expected, the prefix query is improved immensely, with hardly any latency at even 200k(10 attr) devices; that's
at 10 connections, but up to 10-100 concurrent connections are not a problem. The reason is that a prefix
search is *the* case that tree-based indexes are meant to solve.

This was not the case for the 'mac' query - the performance even degraded slightly.
The reason is that it's a very broad query, with no effective prefix to hang on to. As a result,
with e.g. 10k devices - the db engine has to examine 10k index keys to find our 10 devices; then fetch them.
For comparison, in the prefix case - only 11 keys have to be examined.
So, the index here adds only some overhead to a dumb scan.

Still, the numbers are satisfactory at 10 connections and 50k devices, and only fall off dramatically at 100 connections.

To conclude:
The results are promising for our guestimated number of concurrent connections(10), but not many more (50 seems the limit).

Under that assumption, both non-indexed and indexed regexes perform ok up to 50k 'typical' devices.

Indexes are worth pursuing, as for cases they're designed for (prefix) they boost performance considerably.

Prefix queries should be favored or enforced, as for queries that are too open - performance is unpredictable and indexes don't help.

Results have to be taken with a grain of salt given the contrived nature of the microbenchmark.

These estimates will not substitute performance monitoring and reacting to issues on the fly.

# Misc notes

The risks of indexing:
- they run out at 64 indexes
    - each tenant can have max 64 unique attributes, across all devices
    - say - 10 attributes per device gives max 6 separate products (with diff. attributes)
    - when the pool runs out - new attributes will not have an index, so we're back to figures from Run 1
- used only with anchored queries
    - if we expose free-form regexes, users can potentially use non-performant queries
    - maybe we should add an anchor automatically on the backend after all...?

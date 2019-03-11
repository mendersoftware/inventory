This documents a simple microbenchmark created for the regex search functionality. The aim is to see if a naive
implementation will take us anywhere. It assumes no indexes, because we're not sure yet if they're feasible
(hard limit per collection, need data type management - must not conflict; need to be created on the fly for
new fields)

Benchmarking is done indirectly, via http benchmarking (instead of db-level benchmarking). This is because of 
some awkwardness of mongo perf testing tools (need js scripting, and/or old python, and/or done via mongo shell...) - 
no time to figure it out for this POC. The tool of choice is https://github.com/codesenberg/bombardier.

The benchmark metrics are:
- latency(ms)
- req/s

Variables:
- number of devices (1-5000)
- number of attributes per device (1, 5, 10)
- regex query:
    - prefix (`^A1`)
        - will find only anchored values like `A1_foo`
        - the simplest query
    - mac
        - anchored MAC regex (`^$`)
    - infix (`A1`)
        - will find `A1` anywhere, more demanding (full scan)
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
- stuff like page/cache trashing (e.g. between tenants - how is memory used between dbs).

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

## Discussion
Let's assume some limits/rules of thumb we're aiming at; these are based on different numbers floating
around the net. It's a very open question as to what the numbers should be:

- latency: 100-200ms
    - 100ms quoted as 'max comfortable' latency for an actual human user
    - let's make 200ms a stretch value
    - twitter(2008): 200ms

- req/s: 100?
    - stackexachange(2013): 1700
    - twitter(2008) had 200-300
    - slashdot effect said to be(2006): 25-50, but that's pretty old

We seem to be ok on the req/s front, they're always a good couple hundred (breaking point is
at 1k concurrent connections, 5k devs with 10 attr each).

The limiting factor seems to be the latency, and the target range is exceeded at around:
- 5k devs
- 5-10 attributes
- 100 connections

Assuming an average device with 10 attributes, the metrics are decent for the 1k device range.
For the most risky query, the infix, we can go up to almost 500 connections.

I don't want to draw too many conclusions from this, but it seems like:
- for a couple hundred devices
- with an average ~10 attributes
- we should be ok up to a couple hundred connections
- esp. if we limit the queries to prefix

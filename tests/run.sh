#!/bin/bash
cd testing

# if we're running in a container, wait a little before starting tests
[ $$ -eq 1 ] && sleep 5

py.test-3 -s --tb=short --api=0.1.0  --host mender-inventory:8080 --verbose --junitxml=results.xml tests/

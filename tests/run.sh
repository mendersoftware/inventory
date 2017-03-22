#!/bin/bash
cd testing
cp ./docs/management_api.yml .
py.test-3 -s --tb=short --api=0.1.0  --host mender-inventory:8080 --verbose --junitxml=results.xml tests/

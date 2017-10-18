#!/bin/bash

# tests are supposed to be located in the same directory as this file

DIR=$(readlink -f $(dirname $0))

export PYTHONDONTWRITEBYTECODE=1

HOST=${HOST="mender-inventory:8080"}

# if we're running in a container, wait a little before starting tests
[ $$ -eq 1 ] && {
    echo "-- running in container, wait for other services"
    # wait 10s for containters to start and
    sleep 10
}

py.test-3 -s --tb=short --host $HOST \
          --internal-spec $DIR/internal_api.yml \
          --management-spec $DIR/management_api.yml \
          --verbose --junitxml=$DIR/results.xml \
          --inventory-items $DIR/inventory_items \
          $DIR/tests/test_*.py "$@"


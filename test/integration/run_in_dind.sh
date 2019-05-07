#!/bin/bash

set -e

SCRIPT_BASEDIR=$(dirname "$0")
ROOTDIR=$(realpath "$SCRIPT_BASEDIR/../..")

# start Docker daemon
/usr/local/bin/dockerd-entrypoint.sh &

sleep 10

# run tests
$SCRIPT_BASEDIR/run.sh -k -- -v -focus Provider_Secret

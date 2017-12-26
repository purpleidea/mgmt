#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# simple way to kick off runs of the project, since 'go run' sucks!
################################################################################

make build || exit 1
sudo ./mgmt "$@"
e=$?
make clean
exit $e

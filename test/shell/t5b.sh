#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# should take slightly more than 35s, but fail if we take much longer)
$timeout --kill-after=55s 50s ./mgmt run --yaml test/shell/t5b.yaml --converged-timeout=5 --no-watch --no-pgp --tmp-prefix &
pid=$!
wait $pid	# get exit status

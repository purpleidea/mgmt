#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# should take at least 55s, but fail if we block this
# TODO: it would be nice to make sure this test doesn't exit too early!
$timeout --kill-after=120s 110s ./mgmt run --yaml test/shell/sema-1.yaml --sema 2 --converged-timeout=5 --no-watch --no-pgp --tmp-prefix &
pid=$!
wait $pid	# get exit status

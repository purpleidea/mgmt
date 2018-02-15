#!/bin/bash -e

# should take at least 55s, but fail if we block this
# TODO: it would be nice to make sure this test doesn't exit too early!
$timeout --kill-after=120s 110s "$MGMT" run --yaml sema-1.yaml --sema 2 --converged-timeout=5 --no-watch --no-pgp --tmp-prefix &
pid=$!
wait $pid	# get exit status
exit $?

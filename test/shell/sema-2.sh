#!/bin/bash -e

# should take at least 55s, but fail if we block this
# TODO: it would be nice to make sure this test doesn't exit too early!
$timeout --kill-after=360s 300s "$MGMT" run --converged-timeout=5 --no-watch --no-pgp --tmp-prefix lang --lang sema-2.mcl &
pid=$!
wait $pid	# get exit status
exit $?

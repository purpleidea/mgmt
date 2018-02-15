#!/bin/bash -e

# should take slightly more than 35s, but fail if we take much longer)
$timeout --kill-after=55s 50s "$MGMT" run --yaml t5.yaml --converged-timeout=5 --no-watch --no-pgp --tmp-prefix &
pid=$!
wait $pid	# get exit status
exit $?

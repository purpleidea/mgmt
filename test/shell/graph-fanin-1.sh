#!/bin/bash -e

# should take slightly more than 25s, but fail if we take 35s)
$timeout --kill-after=35s 30s ./mgmt run --yaml graph-fanin-1.yaml --converged-timeout=5 --no-watch --tmp-prefix --no-pgp &
pid=$!
wait $pid	# get exit status
exit $?

#!/bin/bash -e

# should take slightly more than 35s, but fail if we take 45s)
timeout --kill-after=45s 40s ./mgmt run --file t5.yaml --converged-timeout=5 --no-watch &
pid=$!
wait $pid	# get exit status
exit $?

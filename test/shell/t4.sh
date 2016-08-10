#!/bin/bash -e

# should take slightly more than 25s, but fail if we take 35s)
timeout --kill-after=35s 30s ./mgmt run --file t4.yaml --converged-timeout=5 --no-watch --allow-tmp-prefix &
pid=$!
wait $pid	# get exit status
exit $?

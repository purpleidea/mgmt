#!/bin/bash -e

# should take 15 seconds for longest resources plus startup time to shutdown
# we don't want the ^C to allow the rest of the graph to continue executing!
$timeout --kill-after=65s 55s "$MGMT" run --yaml graph-exit2.yaml --no-watch --no-pgp --tmp-prefix &
pid=$!
sleep 10s	# let the initial resources start to run...
killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

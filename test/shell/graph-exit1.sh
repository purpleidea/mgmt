#!/bin/bash -e

# should take 15 seconds for longest resources plus startup time to shutdown
# we don't want the ^C to allow the rest of the graph to continue executing!
# this is a test of mgmt exiting quickly via a fast pause after it sees a ^C
$timeout --kill-after=60s 55s "$MGMT" run --yaml graph-exit1.yaml --no-watch --no-pgp --tmp-prefix &
pid=$!
sleep 5s	# let the initial resources start to run...
killall -SIGINT mgmt	# send ^C to exit mgmt
killall -SIGINT mgmt	# send a second ^C to activate fast pause
wait $pid	# get exit status
exit $?

#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# should take 15 seconds for longest resources plus startup time to shutdown
# we don't want the ^C to allow the rest of the graph to continue executing!
$timeout --kill-after=35s 25s ./mgmt run --yaml test/shell/graph-exit1.yaml --no-watch --no-pgp --tmp-prefix &
pid=$!
sleep 5s	# let the initial resources start to run...
killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status

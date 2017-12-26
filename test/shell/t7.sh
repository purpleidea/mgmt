#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# run empty graph
$timeout --kill-after=45s 35s ./mgmt run --tmp-prefix --no-pgp &
pid=$!
sleep 10s	# let it converge
$(sleep 3s && killall -SIGINT mgmt)&	# send ^C to exit mgmt
wait $pid	# get exit status

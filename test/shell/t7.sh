#!/bin/bash -e

# run empty graph
timeout --kill-after=20s 15s ./mgmt run --allow-tmp-prefix &
pid=$!
sleep 5s	# let it converge
$(sleep 3s && killall -SIGINT mgmt)&	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

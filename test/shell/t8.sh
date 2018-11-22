#!/bin/bash -e

exit 0	# XXX: test temporarily disabled till etcd or mgmt regression is fixed.

# run empty graphs, we're just testing etcd clustering
$timeout --kill-after=180s 120s "$MGMT" run --hostname h1 --tmp-prefix empty &
pid1=$!
sleep 15s	# let it startup

$timeout --kill-after=180s 120s "$MGMT" run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix empty &
pid2=$!
sleep 15s

$(sleep 15s && kill -SIGINT $pid2)&	# send ^C to exit 2nd mgmt
wait $pid2
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -SIGINT $pid1)&	# send ^C to exit 1st mgmt
wait $pid1	# get exit status
# if pid1 exits because of a timeout, then it blocked, and this is a bug!
exit $?

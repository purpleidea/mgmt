#!/bin/bash -e

# run empty graphs, we're just testing etcd clustering
$timeout --kill-after=210s 180s "$MGMT" run --hostname h1 --tmp-prefix empty &
pid1=$!
sleep 15s	# let it startup

$timeout --kill-after=210s 180s "$MGMT" run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix empty &
pid2=$!
sleep 15s

$timeout --kill-after=210s 180s "$MGMT" run --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 --tmp-prefix empty &
pid3=$!
sleep 15s

$(sleep 15s && kill -SIGINT $pid3)&	# send ^C to exit 3rd mgmt
wait $pid3
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

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

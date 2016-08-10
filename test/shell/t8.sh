#!/bin/bash -e

# run empty graphs, we're just testing etcd clustering
timeout --kill-after=120s 90s ./mgmt run --hostname h1 --allow-tmp-prefix &
pid1=$!
sleep 5s	# let it startup

timeout --kill-after=120s 90s ./mgmt run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --allow-tmp-prefix &
pid2=$!
sleep 5s

$(sleep 5s && kill -SIGINT $pid2)&	# send ^C to exit 2nd mgmt
wait $pid2
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 5s && kill -SIGINT $pid1)&	# send ^C to exit 1st mgmt
wait $pid1	# get exit status
# if pid1 exits because of a timeout, then it blocked, and this is a bug!
exit $?

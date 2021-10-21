#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

if in_ci github; then
	# TODO: consider debugging this (flaky)
	echo "This is failing in github, skipping test!"
	exit
fi

# run empty graphs, we're just testing etcd clustering
$TIMEOUT "$MGMT" run --hostname h1 --tmp-prefix empty &
pid1=$!
sleep 45s	# let it startup

$TIMEOUT "$MGMT" run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix empty &
pid2=$!
sleep 45s

$(sleep 45s && kill -SIGINT $pid1)&	# send ^C to exit 1st mgmt! (reverse!)
wait $pid1
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 45s && kill -SIGINT $pid2)&	# send ^C to exit 2nd mgmt (reverse!)
wait $pid2	# get exit status
# if pid2 exits because of a timeout, then it blocked, and this is a bug!
exit $?

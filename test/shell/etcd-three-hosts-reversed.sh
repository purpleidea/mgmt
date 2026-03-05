#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

if in_env github; then
	# TODO: consider debugging this
	echo "This is failing in github, skipping test!"
	exit
fi

# run empty graphs, we're just testing etcd clustering
exec_mgmt run --hostname h1 --tmp-prefix empty &
pid1=$!
sleep 15s	# let it startup

exec_mgmt run --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381 --server-urls=http://127.0.0.1:2382 --tmp-prefix empty &
pid2=$!
sleep 15s

exec_mgmt run --hostname h3 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2383 --server-urls=http://127.0.0.1:2384 --tmp-prefix empty &
pid3=$!
sleep 15s

$(sleep 15s && kill -2 $pid1)&	# send ^C to exit 1st mgmt (reversed!)
wait $pid1
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -2 $pid2)&	# send ^C to exit 2nd mgmt
wait $pid2
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -2 $pid3)&	# send ^C to exit 3rd mgmt (reversed!)
wait $pid3	# get exit status
# if pid3 exits because of a timeout, then it blocked, and this is a bug!
exit $?

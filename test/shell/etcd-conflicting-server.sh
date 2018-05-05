#!/bin/bash

. "$(dirname "$0")/../util.sh"

# run empty graphs, we're just testing etcd clustering
$TIMEOUT "$MGMT" run --no-pgp --tmp-prefix empty &
pid1=$!
sleep 15s	# let it startup

# run a second one that should conflict because a server is already running...
$TIMEOUT "$MGMT" run --no-pgp --tmp-prefix empty &
pid2=$!
wait $pid2
e=$?
if [ $e -eq 0 ]; then
	echo "second mgmt exited successfully when error was expected"
	exit 1
fi
if [ $e -ne 1 ]; then
	echo "second mgmt exited with unexpected error of $e"
	exit $e
fi

$(kill -SIGINT $pid1)&	# send ^C to exit 1st mgmt
wait $pid1	# get exit status
# if pid1 exits because of a timeout, then it blocked, and this is a bug!
exit $?

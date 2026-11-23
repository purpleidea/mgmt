#!/usr/bin/env bash

. "$(dirname "$0")/../util.sh"

# run empty graphs, we're just testing etcd clustering
$TIMEOUT "$MGMT" run --no-pgp --tmp-prefix empty &
pid1=$!
sleep 15s	# let it startup

# run a second one that should conflict because a server is already running...
$TIMEOUT "$MGMT" run --no-pgp --tmp-prefix empty
second_status=$?

# Signal timeout's mgmt child directly so timeout reports its exit status.
shutdown_status=0
pkill -INT -P "$pid1" -x "${MGMT##*/}" || shutdown_status=$?
wait "$pid1" || shutdown_status=$?

if [ "$second_status" -eq 0 ]; then
	echo "second mgmt exited successfully when error was expected"
	exit 1
fi
if [ "$second_status" -ne 1 ]; then
	echo "second mgmt exited with unexpected error of $second_status"
	exit "$second_status"
fi

exit "$shutdown_status"

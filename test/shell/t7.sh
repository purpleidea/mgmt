#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

set -x

# run empty graph
exec_mgmt run --tmp-prefix --no-pgp empty &
pid=$!

exithandle() {
	local exitcode=$?
	kill -2 $pid
	wait $pid
	timeout_exitcode=$?
	if [ $exitcode -ne 0 ]; then
		exit $exitcode
	fi
	exit $timeout_exitcode
}
trap 'exithandle' EXIT

sleep 10s	# let it converge

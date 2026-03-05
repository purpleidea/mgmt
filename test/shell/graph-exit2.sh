#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# should take 15 seconds for longest resources plus startup time to shutdown
# we don't want the ^C to allow the rest of the graph to continue executing!
exec_mgmt run --no-watch --no-pgp --tmp-prefix yaml graph-exit2.yaml &
pid=$!
sleep 10s	# let the initial resources start to run...
kill -2 $pid	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

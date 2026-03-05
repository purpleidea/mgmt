#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# should take more than 25s plus overhead
exec_mgmt run --converged-timeout=5 --no-watch --tmp-prefix --no-pgp yaml graph-fanin-1.yaml &
pid=$!
wait $pid	# get exit status
exit $?

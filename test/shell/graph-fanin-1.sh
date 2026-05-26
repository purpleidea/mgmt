#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# should take more than 25s plus overhead
$TIMEOUT "$MGMT" run --converger-timeout=5 --converged-exit --no-watch --tmp-prefix --no-pgp yaml graph-fanin-1.yaml &
pid=$!
wait $pid	# get exit status
exit $?

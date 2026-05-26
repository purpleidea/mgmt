#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# should take slightly more than 35s, but fail if we take much longer)
$TIMEOUT "$MGMT" run --converger-timeout=5 --converged-exit --no-watch --no-pgp --tmp-prefix yaml t5.yaml &
pid=$!
wait $pid	# get exit status
exit $?

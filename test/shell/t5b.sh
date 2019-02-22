#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# should take slightly more than 35s, but fail if we take much longer)
$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --no-pgp --tmp-prefix yaml --yaml t5b.yaml &
pid=$!
wait $pid	# get exit status
exit $?

#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# should take at least 55s, but fail if we block this
# TODO: it would be nice to make sure this test doesn't exit too early!
$TIMEOUT "$MGMT" run  --sema 2 --converged-timeout=5 --no-watch --no-pgp --tmp-prefix yaml sema-1.yaml &
pid=$!
wait $pid	# get exit status
exit $?

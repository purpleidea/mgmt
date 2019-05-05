#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# should take at least 55s, but fail if we block this
# TODO: it would be nice to make sure this test doesn't exit too early!
$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --no-pgp --tmp-prefix lang sema-2.mcl &
pid=$!
wait $pid	# get exit status
exit $?

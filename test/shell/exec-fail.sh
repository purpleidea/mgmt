#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# should take a few seconds plus converged timeout, and test we don't hang!
# TODO: should we return a different exit code if the resources fail?
# TODO: should we be converged if one of the resources has permanently failed?
$TIMEOUT "$MGMT" run --converged-timeout=15 --no-watch --no-pgp --tmp-prefix yaml --yaml exec-fail.yaml &

pid=$!
wait $pid	# get exit status
exit $?

#!/bin/bash -e

# should take a few seconds plus converged timeout, and test we don't hang!
# TODO: should we return a different exit code if the resources fail?
# TODO: should we be converged if one of the resources has permanently failed?
$timeout --kill-after=40s 35s ./mgmt run --yaml exec-fail.yaml --converged-timeout=5 --no-watch --no-pgp --tmp-prefix &
pid=$!
wait $pid	# get exit status
exit $?

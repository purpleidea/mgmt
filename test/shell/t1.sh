#!/bin/bash -e
# NOTES:
#	* this is a simple shell based `mgmt` test case
#	* it is recommended that you run mgmt wrapped in the timeout command
#	* it is recommended that you run mgmt with --no-watch
#	* it is recommended that you run mgmt --converged-timeout=<seconds>
#	* you can run mgmt with --max-runtime=<seconds> in special scenarios

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

$TIMEOUT "$MGMT" --help # hello world!
pid=$!
wait $pid	# get exit status
exit $?

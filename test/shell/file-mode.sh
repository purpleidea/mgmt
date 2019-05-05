#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

set -x

# run till completion
$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix yaml file-mode.yaml &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test $($STAT -c%a /tmp/mgmt/f2) = 741
test $($STAT -c%a /tmp/mgmt/f3) = 614

exit $e

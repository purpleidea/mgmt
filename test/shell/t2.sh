#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# run till completion
$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix yaml t2.yaml &
pid=$!
wait $pid	# get exit status
e=$?

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test ! -e /tmp/mgmt/f4
test -d /tmp/mgmt/dir1

exit $e

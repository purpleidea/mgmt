#!/bin/bash -e

set -x

. ../util.sh

# run till completion
$timeout --kill-after=60s 55s "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix yaml --yaml file-mode.yaml &
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

#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# run till completion
$timeout --kill-after=40s 35s ./mgmt run --yaml test/shell/file-mode.yaml --converged-timeout=5 --no-watch --tmp-prefix &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test $(stat -c%a /tmp/mgmt/f2) = 741
test $(stat -c%a /tmp/mgmt/f3) = 614

exit $e

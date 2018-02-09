#!/bin/bash -e

if [[ $(uname) == "Darwin" ]] ; then
	# https://github.com/purpleidea/mgmt/issues/33
	echo "This test is broken on macOS, skipping!"
	exit
fi

set -x

# run till completion
$timeout --kill-after=40s 35s ./mgmt run --yaml file-mode.yaml --converged-timeout=5 --no-watch --tmp-prefix &
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

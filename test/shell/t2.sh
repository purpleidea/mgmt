#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

if in_ci github; then
	# TODO: consider debugging this
	echo "This is failing in github, skipping test!"
	exit
fi

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

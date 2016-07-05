#!/bin/bash -e

if env | grep -q -e '^TRAVIS=true$'; then
	# inotify doesn't seem to work properly on travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

# run till completion
timeout --kill-after=15s 10s ./mgmt run --file t2.yaml --converged-timeout=5 --no-watch &

. wait.sh	# wait for mgmt

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test ! -e /tmp/mgmt/f4

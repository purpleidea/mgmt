#!/bin/bash -e

if env | grep -q -e '^TRAVIS=true$'; then
	# inotify doesn't seem to work properly on travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

# run till completion
timeout --kill-after=20s 15s ./mgmt run --file t6.yaml --no-watch &
sleep 1s	# let it converge
test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test ! -e /tmp/mgmt/f4
rm -f /tmp/mgmt/f2
sleep 0.1s	# let it converge or tests will fail
test -e /tmp/mgmt/f2
rm -f /tmp/mgmt/f2
sleep 0.1s
test -e /tmp/mgmt/f2
echo foo > /tmp/mgmt/f2
sleep 0.1s
test "`cat /tmp/mgmt/f2`" = "i am f2"
rm -f /tmp/mgmt/f2
sleep 0.1s
test -e /tmp/mgmt/f2

killall -SIGINT mgmt	# send ^C to exit mgmt

. wait.sh	# wait for mgmt

#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# run till completion
exec_mgmt run --no-watch --tmp-prefix yaml t6.yaml &
pid=$!

exithandle() {
	local exitcode=$?
	kill -2 $pid
	wait $pid
	timeout_exitcode=$?
	if [ $exitcode -ne 0 ]; then
		exit $exitcode
	fi
	exit $timeout_exitcode
}
trap 'exithandle' EXIT

sleep 60s	# let it converge
test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test ! -e /tmp/mgmt/f4
rm -f /tmp/mgmt/f2
sleep 1s	# let it converge or tests will fail
test -e /tmp/mgmt/f2
rm -f /tmp/mgmt/f2
sleep 1s
test -e /tmp/mgmt/f2
echo foo > /tmp/mgmt/f2
sleep 1s
test "`cat /tmp/mgmt/f2`" = "i am f2"
rm -f /tmp/mgmt/f2
sleep 1s
test -e /tmp/mgmt/f2

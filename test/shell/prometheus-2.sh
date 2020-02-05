#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

TEMPFILE=`mktemp mgmt-tests-XXXXXXXX`

cleanup()
{
	killall -SIGINT mgmt	# send ^C to exit mgmt
	rm -f $TEMPFILE
	wait $pid	# get exit status
}

grep_or_fail()
{
	cat >$TEMPFILE
	grep "$1" $TEMPFILE && return
	echo >&2 "ERROR: expected pattern '$1' not found"
	echo >&2 "output was:"
	cat >&2 $TEMPFILE
	exit 1
}

# run empty graph, with prometheus support
$TIMEOUT "$MGMT" run --tmp-prefix --prometheus --prometheus-listen :52637 empty &
pid=$!
trap cleanup EXIT

sleep 5s	# let it converge

# TODO: Find out why this is not happening anymore, or remove this particular check
# Check that etcd metrics are loaded
#curl -s 127.0.0.1:52637/metrics | grep_or_fail "^etcd_server_has_leader 1"

# Check that go metrics are loaded
curl -s 127.0.0.1:52637/metrics | grep_or_fail "^go_goroutines [0-9]\+"

trap - EXIT
cleanup
exit $?

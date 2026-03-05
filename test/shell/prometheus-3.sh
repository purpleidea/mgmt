#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

exit 0 # XXX: temporarily disabled until prometheus is added back post refactor

TEMPFILE=`mktemp mgmt-tests-XXXXXXXX`

cleanup()
{
	local exitcode=$?
	rm -f $TEMPFILE
	kill -2 $pid
	wait $pid
	local timeout_exitcode=$?
	if [ $exitcode -ne 0 ]; then
		exit $exitcode
	fi
	exit $timeout_exitcode
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

# run a graph, with prometheus support
exec_mgmt run --tmp-prefix --no-pgp --prometheus yaml prometheus-3.yaml &
pid=$!
trap 'cleanup' EXIT

sleep 10s	# let it converge

# For test debugging purpose
curl 127.0.0.1:9233/metrics

# Three CheckApply for a File ; with events
curl 127.0.0.1:9233/metrics | grep_or_fail '^mgmt_checkapply_total{apply="true",errorful="false",eventful="true",kind="file"} 3$'

# One CheckApply for a File ; in noop mode.
curl 127.0.0.1:9233/metrics | grep_or_fail '^mgmt_checkapply_total{apply="false",errorful="false",eventful="true",kind="file"} 1$'

# Check mgmt_graph_start_time_seconds
curl 127.0.0.1:9233/metrics | grep_or_fail "^mgmt_graph_start_time_seconds [1-9]\+"

#!/bin/bash -e

if env | grep -q -e '^TRAVIS=true$'; then
	# inotify doesn't seem to work properly on travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

mkdir -p "${MGMT_TMPDIR}"
> "${MGMT_TMPDIR}"sshd_config

# run empty graph, with prometheus support
$timeout --kill-after=60s 55s "$MGMT" run --tmp-prefix --yaml=augeas-1.yaml &
pid=$!

# kill server on error
trap 'kill -SIGINT "$pid"' EXIT

sleep 10s	# let it converge

# make an exception on macOS as augeas behaves differently
if [[ $(uname) == "Darwin" ]] ; then
	value=false
else
	value=no
fi

# make it easier to see why the test failed
set -x
cat "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

sed -i '' "s/${value}/yes/" "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding yes" "${MGMT_TMPDIR}"sshd_config

sleep 10s	# Augeas is slow

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

trap '' EXIT
killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

#!/bin/bash -e

mkdir -p "${MGMT_TMPDIR}"
echo > "${MGMT_TMPDIR}"sshd_config

$timeout --kill-after=360s 300s "$MGMT" run --tmp-prefix yaml --yaml=augeas-1.yaml &
pid=$!

# kill server on error
trap 'kill -SIGINT "$pid"' EXIT

sleep 30s	# let it converge

# make an exception on macOS as augeas behaves differently
if [[ $(uname) == "Darwin" ]] ; then
	value=false
else
	#value=no
	value=false	# seems it's this on linux now too
fi

# make it easier to see why the test failed
set -x
cat "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

sed -i 's/'"${value}"'/yes/' "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding yes" "${MGMT_TMPDIR}"sshd_config

sleep 10s	# augeas is slow

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

trap '' EXIT
killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

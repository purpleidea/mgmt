#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

mkdir -p "${MGMT_TMPDIR}"
echo > "${MGMT_TMPDIR}"sshd_config

exec_mgmt run --tmp-prefix yaml augeas-1.yaml &
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

sleep 30s	# let it converge

value=false

# make it easier to see why the test failed
set -x
cat "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

sed -i.bak 's/'"${value}"'/yes/' "${MGMT_TMPDIR}"sshd_config && rm "${MGMT_TMPDIR}"sshd_config.bak

grep "X11Forwarding yes" "${MGMT_TMPDIR}"sshd_config

sleep 10s	# augeas is slow

grep "X11Forwarding ${value}" "${MGMT_TMPDIR}"sshd_config

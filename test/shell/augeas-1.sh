#!/bin/bash -e

if env | grep -q -e '^TRAVIS=true$'; then
	# inotify doesn't seem to work properly on travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

mkdir -p "${MGMT_TMPDIR}"
> "${MGMT_TMPDIR}"sshd_config

# run empty graph, with prometheus support
timeout --kill-after=20s 15s ./mgmt run --tmp-prefix --yaml=augeas-1.yaml &
pid=$!
sleep 5s	# let it converge

grep "X11Forwarding no" "${MGMT_TMPDIR}"sshd_config

sed -i "s/no/yes/" "${MGMT_TMPDIR}"sshd_config

grep "X11Forwarding yes" "${MGMT_TMPDIR}"sshd_config

sleep 3	# Augeas is slow

grep "X11Forwarding no" "${MGMT_TMPDIR}"sshd_config


killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

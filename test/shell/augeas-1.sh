#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

if env | grep -q -e '^TRAVIS=true$'; then
	# inotify doesn't seem to work properly on travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

readonly config_file="${MGMT_TMPDIR}/"sshd_config

mkdir -p "${MGMT_TMPDIR}" || :
> "${config_file}"

# run empty graph, with prometheus support
timeout --kill-after=40s 35s ./mgmt run --tmp-prefix --yaml=test/shell/augeas-1.yaml &
pid=$!
sleep 5s	# let it converge

grep "X11Forwarding no" "${config_file}"

sed -i "s/no/yes/" "${config_file}"

grep "X11Forwarding yes" "${config_file}"

sleep 3	# Augeas is slow

grep "X11Forwarding no" "${config_file}"


killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status

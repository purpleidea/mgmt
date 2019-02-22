#!/bin/bash

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

if ! ifconfig lo | grep 'inet6 ::1' >/dev/null; then
	echo "No IPv6, skipping test"
	exit 0
fi

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

# run empty graph listing only to IPv6 addresses
$TIMEOUT "$MGMT" run --client-urls "http://[::1]:2379" --server-urls "http://[::1]:2380" --tmp-prefix empty &
pid=$!

# kill server on error/exit
trap 'pkill -9 mgmt' EXIT

# give mgmt a little time to startup
sleep 10s

# mgmt configured for ipv6 only should not listen on any IPv4 ports
lsof -Pn -p "$pid" -a -i | grep '127.0.0.1' && false

# instead it should listen on IPv6
lsof -Pn -p "$pid" -a -i | grep '::1' || false

#!/usr/bin/env bash

. "$(dirname "$0")/../util.sh"

if in_env github; then
	# TODO: consider debugging this (flaky)
	echo "This is failing in github, skipping test!"
	exit
fi

# Tests the behaviour of the --no-network
set -o errexit
set -o pipefail

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

# run empty graph, with standalone enabled
$TIMEOUT "$MGMT" run --no-network --prefix "$tmpdir" empty &
pid=$!

# kill server on error/exit
trap 'kill -SIGINT "$pid"' EXIT

# give mgmt a little time to startup
sleep 10

# standalone mgmt should not listen on any tcp ports
lsof -i | grep "$pid" | grep TCP && false

# instead unix domain sockets should have been created
test -S "servers.sock:0"
test -S "clients.sock:0"

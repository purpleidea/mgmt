#!/bin/bash

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

$TIMEOUT "$MGMT" run --hostname h1 --tmp-prefix --no-pgp empty &
pid1=$!
sleep 10s
$TIMEOUT "$MGMT" run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix --no-pgp empty &
pid2=$!
sleep 10s
$TIMEOUT "$MGMT" run --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 --tmp-prefix --no-pgp empty &
pid3=$!
sleep 10s
$TIMEOUT "$MGMT" run --hostname h4 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 --tmp-prefix --no-pgp empty &
pid4=$!
sleep 10s
$TIMEOUT "$MGMT" deploy --no-git --seeds http://127.0.0.1:2379 lang --lang exchange0.mcl

# kill servers on error/exit
#trap 'pkill -9 mgmt' EXIT

# wait for everything to converge
sleep 15s

# debug
tail /tmp/mgmt/exchange-*

test "$(cat /tmp/mgmt/exchange-* | grep -c h1)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h2)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h3)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h4)" -eq 4

$(sleep 15s && kill -SIGINT $pid4)&	# send ^C to exit mgmt...
wait $pid4
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -SIGINT $pid3)&	# send ^C to exit mgmt...
wait $pid3
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -SIGINT $pid2)&	# send ^C to exit mgmt...
wait $pid2
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

$(sleep 15s && kill -SIGINT $pid1)&	# send ^C to exit mgmt...
wait $pid1
e=$?
if [ $e -ne 0 ]; then
	exit $e
fi

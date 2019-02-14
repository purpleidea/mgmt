#!/bin/bash -e

# run empty graph, with prometheus support
$timeout --kill-after=360s 300s "$MGMT" run --tmp-prefix --prometheus empty &
pid=$!
sleep 5s	# let it converge

# Check that etcd metrics are loaded
curl 127.0.0.1:9233/metrics | grep "^etcd_server_has_leader 1"

# Check that go metrics are loaded
curl 127.0.0.1:9233/metrics | grep "^go_goroutines [0-9]\+"

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

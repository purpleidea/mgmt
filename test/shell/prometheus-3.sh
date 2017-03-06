#!/bin/bash -e

# run a graph, with prometheus support
timeout --kill-after=30s 25s ./mgmt run --tmp-prefix --no-pgp --prometheus --yaml prometheus-3.yaml &
pid=$!
sleep 10s	# let it converge

# For test debugging purpose
curl 127.0.0.1:9233/metrics

# Three CheckApply for a File ; with events
curl 127.0.0.1:9233/metrics | grep '^mgmt_checkapply_total{apply="true",errorful="false",eventful="true",kind="file"} 3$'

# One CheckApply for a File ; in noop mode.
curl 127.0.0.1:9233/metrics | grep '^mgmt_checkapply_total{apply="false",errorful="false",eventful="true",kind="file"} 1$'

# Check mgmt_graph_start_time_seconds
curl 127.0.0.1:9233/metrics | grep "^mgmt_graph_start_time_seconds [1-9]\+"

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

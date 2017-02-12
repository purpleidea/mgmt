#!/bin/bash -e

# run empty graph, with prometheus support
timeout --kill-after=20s 15s ./mgmt run --tmp-prefix --prometheus --yaml prometheus-3.yaml &
pid=$!
sleep 10s	# let it converge

# For test debugging purpose
curl 127.0.0.1:9233/metrics

# Three CheckApply for a File ; with events
curl 127.0.0.1:9233/metrics | grep '^mgmt_checkapply_total{apply="true",errorful="false",eventful="true",kind="File"} 3$'

# One CheckApply for a File ; in noop mode.
curl 127.0.0.1:9233/metrics | grep '^mgmt_checkapply_total{apply="false",errorful="false",eventful="true",kind="File"} 1$'


killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

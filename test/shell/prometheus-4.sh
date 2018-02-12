#!/bin/bash -xe

if [[ $(uname) == "Darwin" ]] ; then
	# https://github.com/purpleidea/mgmt/issues/33
	echo "This test is broken on macOS, skipping!"
	exit
fi

# run a graph, with prometheus support
$timeout --kill-after=60s 55s ./mgmt run --tmp-prefix --no-pgp --prometheus --yaml prometheus-4.yaml &
pid=$!
sleep 15s	# let it converge

# For test debugging purpose
curl 127.0.0.1:9233/metrics

# Check for mgmt_resources
curl 127.0.0.1:9233/metrics | grep '^mgmt_resources{kind="file"} 4$'

# One CheckApply for a File ; in noop mode.
curl 127.0.0.1:9233/metrics | grep 'mgmt_checkapply_total{apply="false",errorful="false",eventful="true",kind="file"} 1$'

# Two CheckApply for a File ; without errors, with events
curl 127.0.0.1:9233/metrics | grep 'mgmt_checkapply_total{apply="true",errorful="false",eventful="true",kind="file"} 2$'

# Multiple CheckApplies with errors
curl 127.0.0.1:9233/metrics | grep 'mgmt_checkapply_total{apply="true",errorful="true",eventful="true",kind="file"} [0-9]\+'

# One soft failure ATM
curl 127.0.0.1:9233/metrics | grep 'mgmt_failures{failure="soft",kind="file"} 1$'

# Multiple soft failures since startup
if curl 127.0.0.1:9233/metrics | grep 'mgmt_failures_total{failure="soft",kind="file"} 1$'
then
	false
fi
curl 127.0.0.1:9233/metrics | grep 'mgmt_failures_total{failure="soft",kind="file"} [0-9]\+'

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

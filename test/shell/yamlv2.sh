#!/bin/bash -e

# should take slightly more than 25s, but fail if we take 35s)
$timeout --kill-after=10s 8s ./mgmt run --yaml yamlv2.yaml --converged-timeout=1 --no-watch --tmp-prefix &
pid=$!
wait $pid	# get exit status
res=$?

set -ex

stat -c'%Y %n' /tmp/mgmt/*

test /tmp/mgmt/d0 -ot /tmp/mgmt/d1
test /tmp/mgmt/d1 -ot /tmp/mgmt/d2

exit $res

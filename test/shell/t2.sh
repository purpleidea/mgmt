#!/bin/bash

. etcd.sh	# start etcd as job # 1

# run till completion
timeout --kill-after=15s 10s ./mgmt run --file t2.yaml --converged-timeout=5 --no-watch &

#jobs	# etcd is 1
wait -n 2	# wait for mgmt to exit

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test -e /tmp/mgmt/f3
test ! -e /tmp/mgmt/f4

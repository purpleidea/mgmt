#!/bin/bash -e

. etcd.sh	# start etcd as job # 1

# should take slightly more than 25s, but fail if we take 35s)
timeout --kill-after=35s 30s ./mgmt run --file t4.yaml --converged-timeout=5 --no-watch &

#jobs	# etcd is 1
#wait -n 2	# wait for mgmt to exit
. wait.sh	# wait for everything except etcd

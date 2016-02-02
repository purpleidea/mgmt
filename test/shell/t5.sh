#!/bin/bash

. etcd.sh	# start etcd as job # 1

# should take slightly more than 35s, but fail if we take 45s)
timeout --kill-after=45s 40s ./mgmt run --file t5.yaml --converged-timeout=5 --no-watch &

#jobs	# etcd is 1
#wait -n 2	# wait for mgmt to exit
. wait.sh	# wait for everything except etcd

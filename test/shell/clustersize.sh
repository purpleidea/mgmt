#!/bin/bash

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

if ! command -v etcdctl >/dev/null; then
	echo "Missing etcdctl, skipping"
	exit 0
fi

mkdir /tmp/mgmt/{A..E}

# kill servers on error/exit
trap 'pkill -9 mgmt' EXIT

$TIMEOUT "$MGMT" run --hostname h1 --tmp-prefix --no-pgp empty &
$TIMEOUT "$MGMT" run --hostname h2 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 empty &
$TIMEOUT "$MGMT" run --hostname h3 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 empty &

# wait for everything to converge
sleep 30s

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/idealClusterSize 3

$TIMEOUT "$MGMT" run --hostname h4 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 empty &
$TIMEOUT "$MGMT" run --hostname h5 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2387 --server-urls http://127.0.0.1:2388 empty &

# wait for everything to converge
sleep 30s

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list | wc -l)" -eq 3

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/idealClusterSize 5

# wait for everything to converge
sleep 30s

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list | wc -l)" -eq 5

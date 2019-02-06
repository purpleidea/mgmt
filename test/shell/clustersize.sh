#!/bin/bash

set -o errexit
set -o pipefail

if ! command -v etcdctl >/dev/null; then
	echo "Missing etcdctl, skipping"
	exit 0
fi

. "$(dirname "$0")/../util.sh"

mkdir /tmp/mgmt/{A..E}

# kill servers on error/exit
trap 'pkill -9 mgmt' EXIT

"$MGMT" run --hostname h1 --tmp-prefix --no-pgp empty &
"$MGMT" run --hostname h2 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 empty &
"$MGMT" run --hostname h3 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 empty &

# wait for everything to converge
sleep 10

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/idealClusterSize 3

"$MGMT" run --hostname h4 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 empty &
"$MGMT" run --hostname h5 --tmp-prefix --no-pgp --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2387 --server-urls http://127.0.0.1:2388 empty &

# wait for everything to converge
sleep 10

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list | wc -l)" -eq 3

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 put /_mgmt/idealClusterSize 5

# wait for everything to converge
sleep 5

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list | wc -l)" -eq 5

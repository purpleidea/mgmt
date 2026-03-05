#!/usr/bin/env bash

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

if ! command -v etcdctl >/dev/null; then
	echo "Missing etcdctl, skipping"
	exit 0
fi

#mkdir /tmp/mgmt/{A..E}

# kill servers on error/exit
trap 'pkill -f -signal kill "$MGMT"' EXIT

exec_mgmt run --hostname h1 --tmp-prefix --no-pgp empty &
exec_mgmt run --hostname h2 --tmp-prefix --no-pgp --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381 --server-urls=http://127.0.0.1:2382 empty &
exec_mgmt "$MGMT" run --hostname h3 --tmp-prefix --no-pgp --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2383 --server-urls=http://127.0.0.1:2384 empty &

# wait for everything to converge
sleep 30s

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/chooser/dynamicsize/idealclustersize 3

exec_mgmt run --hostname h4 --tmp-prefix --no-pgp --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2385 --server-urls=http://127.0.0.1:2386 empty &
exec_mgmt run --hostname h5 --tmp-prefix --no-pgp --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2387 --server-urls=http://127.0.0.1:2388 empty &

# wait for everything to converge
sleep 30s

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 member list | wc -l)" -eq 3

ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2379 put /_mgmt/chooser/dynamicsize/idealclustersize 5

# wait for everything to converge
sleep 30s

test "$(ETCDCTL_API=3 etcdctl --endpoints 127.0.0.1:2381 member list | wc -l)" -eq 5

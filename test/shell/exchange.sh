#!/bin/bash

set -o errexit
set -o pipefail

. "$(dirname "$0")/../util.sh"

"$MGMT" run --hostname h1 --ideal-cluster-size 1 --tmp-prefix --no-pgp lang --lang exchange0.mcl &
"$MGMT" run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix --no-pgp lang --lang exchange0.mcl &
"$MGMT" run --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 --tmp-prefix --no-pgp lang --lang exchange0.mcl &
"$MGMT" run --hostname h4 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2385 --server-urls http://127.0.0.1:2386 --tmp-prefix --no-pgp lang --lang exchange0.mcl &

# kill servers on error/exit
trap 'pkill -9 mgmt' EXIT

# wait for everything to converge
sleep 10

test "$(cat /tmp/mgmt/exchange-* | grep -c h1)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h2)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h3)" -eq 4
test "$(cat /tmp/mgmt/exchange-* | grep -c h4)" -eq 4

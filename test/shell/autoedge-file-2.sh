#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# Test autoedge with a deeper directory tree (3 levels plus files at each
# level). This exercises the algorithm with more vertices and verifies that
# autoedge ordering is correct even when transitive paths exist.
$TIMEOUT "$MGMT" run --converged-timeout=15 --no-watch --tmp-prefix yaml autoedge-file-2.yaml &
pid=$!
wait $pid
e=$?

test -d /tmp/mgmt/ae2/
test -d /tmp/mgmt/ae2/mid/
test -d /tmp/mgmt/ae2/mid/deep/
test -e /tmp/mgmt/ae2/f3
test -e /tmp/mgmt/ae2/mid/f2
test -e /tmp/mgmt/ae2/mid/deep/f1
test "$(cat /tmp/mgmt/ae2/mid/deep/f1)" = "deep file"
test "$(cat /tmp/mgmt/ae2/mid/f2)" = "mid file"
test "$(cat /tmp/mgmt/ae2/f3)" = "root file"

exit $e

#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# Test that autoedge correctly creates directory hierarchy dependencies for
# file resources. No explicit edges are defined; autoedge should add them so
# parent directories are created before their contents.
$TIMEOUT "$MGMT" run --converged-timeout=15 --no-watch --tmp-prefix yaml autoedge-file-1.yaml &
pid=$!
wait $pid
e=$?

test -d /tmp/mgmt/aedir/
test -d /tmp/mgmt/aedir/subdir/
test -e /tmp/mgmt/aedir/f1
test -e /tmp/mgmt/aedir/subdir/f2
test "$(cat /tmp/mgmt/aedir/f1)" = "hello from f1"
test "$(cat /tmp/mgmt/aedir/subdir/f2)" = "hello from f2"

exit $e

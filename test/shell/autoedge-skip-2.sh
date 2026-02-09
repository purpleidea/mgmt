#!/usr/bin/env -S bash -e
# Test that autoedge skips adding a transitively redundant edge.
# d1/ is the parent of both d1/f1 and d1/f2. Explicit edges d1 -> f1 and
# f1 -> f2 create a path d1 -> f1 -> f2. When autoedge discovers d1 -> f2
# (parent directory relationship), it should skip it because d1 can already
# reach f2 through the existing path. This exercises the isReachable check.

. "$(dirname "$0")/../util.sh"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix --no-pgp yaml autoedge-skip-2.yaml > "${tmpdir}/mgmt.log" 2>&1
e=$?

# verify resources converged
test -d /tmp/mgmt/d1/ || fail_test "directory /tmp/mgmt/d1/ was not created"
test -e /tmp/mgmt/d1/f1 || fail_test "file /tmp/mgmt/d1/f1 was not created"
test -e /tmp/mgmt/d1/f2 || fail_test "file /tmp/mgmt/d1/f2 was not created"

# verify autoedge ran
grep -q "autoedge: building" "${tmpdir}/mgmt.log" || fail_test "autoedge did not run"

# verify autoedge did NOT add any edges (d1 -> f1 is skipped because it
# already exists, and d1 -> f2 is skipped because d1 -> f1 -> f2 exists)
if grep -q "autoedge: adding:" "${tmpdir}/mgmt.log"; then
	echo "mgmt output:"
	cat "${tmpdir}/mgmt.log"
	fail_test "autoedge added a redundant edge that should have been skipped"
fi

# verify the graph has exactly 2 edges (the explicit ones, no autoedge added)
grep -q "Edges(2)" "${tmpdir}/mgmt.log" || fail_test "expected 2 edges in graph"

rm -rf "${tmpdir}"
exit $e

#!/usr/bin/env -S bash -e
# Test that autoedge skips adding an edge that already exists explicitly.
# The directory d1/ is the parent of d1/f1, so autoedge discovers d1 -> f1.
# Since we declare that edge explicitly, autoedge should skip adding it.

. "$(dirname "$0")/../util.sh"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

$TIMEOUT "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix --no-pgp yaml autoedge-skip-1.yaml > "${tmpdir}/mgmt.log" 2>&1
e=$?

# verify resources converged
test -d /tmp/mgmt/d1/ || fail_test "directory /tmp/mgmt/d1/ was not created"
test -e /tmp/mgmt/d1/f1 || fail_test "file /tmp/mgmt/d1/f1 was not created"
grep -q "hello from autoedge skip test" /tmp/mgmt/d1/f1 || fail_test "file content mismatch"

# verify autoedge ran
grep -q "autoedge: building" "${tmpdir}/mgmt.log" || fail_test "autoedge did not run"

# verify autoedge did NOT add a redundant edge (the explicit edge should be
# preserved and autoedge should skip its duplicate discovery)
if grep -q "autoedge: adding:" "${tmpdir}/mgmt.log"; then
	echo "mgmt output:"
	cat "${tmpdir}/mgmt.log"
	fail_test "autoedge added a redundant edge that should have been skipped"
fi

rm -rf "${tmpdir}"
exit $e

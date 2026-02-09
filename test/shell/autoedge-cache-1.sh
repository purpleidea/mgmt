#!/usr/bin/env -S bash -e
# Test that autoedge delta caching replays cached edges on identical graph
# re-evaluations. We use datetime.now() to drive reactive re-evaluations
# where the graph structure stays the same (same resources, no explicit
# edges), so the second autoedge pass should hit the cache and log
# "replaying" instead of "building".

. "$(dirname "$0")/../util.sh"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

# Run mgmt in the background. datetime.now() ticks every second, producing
# new graphs with the same structure. We poll the log for the "replaying"
# message and then kill mgmt rather than relying on converged-timeout
# (which never fires because the reactive graph keeps changing).
$TIMEOUT "$MGMT" run --tmp-prefix --no-pgp lang autoedge-cache-1.mcl > "${tmpdir}/mgmt.log" 2>&1 &
pid=$!

# Wait for the cache replay message to appear (up to 30 seconds).
for i in $(seq 1 30); do
	if grep -q "autoedge: replaying" "${tmpdir}/mgmt.log" 2>/dev/null; then
		break
	fi
	sleep 1s
done

killall -SIGINT mgmt || true	# send ^C to exit mgmt
wait $pid || true	# collect exit status (may be non-zero from signal)

# verify resources converged
test -d /tmp/mgmt/autoedge-cache/ || fail_test "directory was not created"
test -e /tmp/mgmt/autoedge-cache/f1 || fail_test "file was not created"

# verify autoedge ran at least once (first pass does full computation)
grep -q "autoedge: building" "${tmpdir}/mgmt.log" || fail_test "autoedge did not run"

# verify autoedge cache was used on a subsequent pass
grep -q "autoedge: replaying" "${tmpdir}/mgmt.log" || {
	echo "mgmt output:"
	cat "${tmpdir}/mgmt.log"
	fail_test "autoedge cache was never used (no 'replaying' in log)"
}

rm -rf "${tmpdir}"

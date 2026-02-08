#!/usr/bin/env bash
# Regression test for https://github.com/purpleidea/mgmt/issues/842, ensuring a simple user resource can apply twice without error.

. "$(dirname "$0")/../util.sh"

set -o pipefail

if ! timeout 1s sudo -A true; then
	echo "sudo disabled: skipping user re-reconcile test"
	exit
fi

USERNAME="mgmt-test-rereconcile"

cleanup() {
	sudo -A userdel -f "$USERNAME" 2>/dev/null || true
}

# ensure the user doesn't exist before the test
cleanup

# cleanup on exit
trap cleanup EXIT

$TIMEOUT sudo -A "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix lang user-rereconcile.mcl
e=$?
if [ $e -ne 0 ]; then
	fail_test "First run failed with exit code $e"
fi

if ! id "$USERNAME" >/dev/null 2>&1; then
	fail_test "User $USERNAME was not created"
fi

$TIMEOUT sudo -A "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix lang user-rereconcile.mcl
e=$?
if [ $e -ne 0 ]; then
	fail_test "Second run (re-reconciliation) failed with exit code $e"
fi

exit 0

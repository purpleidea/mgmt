#!/bin/bash -e
# vim: noet:ts=8:sts=8:sw=8

. "$(dirname "$0")/../util.sh"

set -x

echo "skipping test as we have no replacement for --converged-timeout (see #743)"
exit 0

assert_sudo "not checking mount"

# run till completion
$TIMEOUT sudo -A "$MGMT" run --converged-timeout=15 --no-watch --tmp-prefix lang mount0.mcl &
pid=$!
wait $pid	# get exit status

mountpoint="/tmp/mgmt.test.mount"
unit="$( systemd-escape --path $mountpoint ).mount"

test -d "$mountpoint" || fail_test "expected directory $mountpoint to be created"
systemctl status "$unit" || fail_test "systemd unit $unit is not running"
grep "$mountpoint" /proc/mounts || fail_test "did not find $mountpoint among mounts"

##### mount tests complete, do unmount in the same test

# run till completion
$TIMEOUT sudo -A "$MGMT" run --converged-timeout=15 --no-watch --tmp-prefix lang umount0.mcl &
pid=$!
wait $pid	# get exit status

systemctl status "$unit" && fail_test "after umount, systemd unit $unit is still running"
grep "$mountpoint" /proc/mounts && fail_test "after umount, $mountpoint still among mounts"

exit 0

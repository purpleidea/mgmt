#!/bin/bash -e
# vim: noet:ts=8:sts=8:sw=8

exit 0	# XXX: disable for now

. "$(dirname "$0")/../util.sh"

set -x

assert_sudo "not checking file owner and group"

# run till completion
$TIMEOUT sudo -A "$MGMT" run  --converged-timeout=5 --no-watch --tmp-prefix yaml file-owner.yaml &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test $(stat -c%U:%G /tmp/mgmt/f1) = root:root
test $(stat -c%u:%g /tmp/mgmt/f2) = 1:2

exit $e

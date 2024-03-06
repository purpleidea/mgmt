#!/bin/bash -e
# vim: noet:ts=8:sts=8:sw=8

. "$(dirname "$0")/../util.sh"

set -x

assert_sudo "not checking file source"

# run till completion
$TIMEOUT sudo -A "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix lang file-source.mcl &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/file-source.txt
cmp --silent file-source.txt /tmp/mgmt/file-source.txt || exit 1

exit $e

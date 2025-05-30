#!/usr/bin/env -S bash -e
# vim: noet:ts=8:sts=8:sw=8

. "$(dirname "$0")/../util.sh"

set -x

if ! timeout 1s sudo -A true; then
	echo "sudo disabled: not checking file owner and group"
	exit
fi

# run till completion
$TIMEOUT sudo -A "$MGMT" run --converged-timeout=5 --no-watch --tmp-prefix lang file-source.mcl &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/file-source.txt
cmp --silent file-source.txt /tmp/mgmt/file-source.txt || exit 1

exit $e

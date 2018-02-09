#!/bin/bash -e
# vim: noet:ts=8:sts=8:sw=8

set -x

if ! timeout 1s sudo -A true; then
	echo "sudo disabled: not checking file owner and group"
	exit
fi

# run till completion
$timeout --kill-after=30s 25s sudo -A ./mgmt run --yaml file-owner.yaml --converged-timeout=5 --no-watch --tmp-prefix &
pid=$!
wait $pid	# get exit status
e=$?

ls -l /tmp/mgmt

test -e /tmp/mgmt/f1
test -e /tmp/mgmt/f2
test $(stat -c%U:%G /tmp/mgmt/f1) = root:root
test $(stat -c%u:%g /tmp/mgmt/f2) = 1:2

exit $e

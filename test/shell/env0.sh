#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

set -o errexit
set -o pipefail

# these values represent environment variable values below or defaults set in test/shell/env0.mcl
regex="123,,:123,321,:true,false:123"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"
if [[ ! "$tmpdir" =~ "/tmp" ]]; then
	echo "unexpected tmpdir in: ${tmpdir}"
	exit 99
fi

env TMPDIR="${tmpdir}" TEST=123 EMPTY="" $TIMEOUT "$MGMT" run --tmp-prefix --converged-timeout=5 lang env0.mcl
e=$?

egrep "$regex" "$tmpdir/environ" || fail_test "Could not match '$(cat "$tmpdir/environ")' in '$tmpdir/environ' to '$regex'."

if [ "$tmpdir" = "" ]; then
	echo "BUG, tried to delete empty string path"
	exit 99
fi
# cleanup if everything went well
rm -r "$tmpdir"

exit $e

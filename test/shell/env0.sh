#!/bin/bash -e

set -o errexit
set -o pipefail

. ../util.sh

# these values represent environment variable values below or defaults set in test/shell/env0.mcl
regex="123,,:123,321,:true,false:123"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

env TMPDIR="${tmpdir}" TEST=123 EMPTY="" $timeout -sKILL 60s "$MGMT" run --tmp-prefix --converged-timeout=5 --lang env0.mcl
e=$?

egrep "$regex" "$tmpdir/environ" || fail_test "Could not match '$(cat "$tmpdir/environ")' in '$tmpdir/environ' to '$regex'."

# cleanup if everything went well
rm -r "$tmpdir"

exit $e

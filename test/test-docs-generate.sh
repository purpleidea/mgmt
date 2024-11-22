#!/bin/bash
# check that our documentation still generates, even if we don't use it here

# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

failures=''

run-test ./mgmt docs generate --output /tmp/docs.json &> /dev/null || fail_test "could not generate: $file"

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo "The following tests have failed:"
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

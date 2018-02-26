#!/bin/bash
# validate the examples using mgmt validate

set -e

# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

failures=''

# validate .mcl examples
for file in $(find examples/lang/ -maxdepth 3 -type f -name '*.mcl'); do
	$MGMT validate --lang "$file" || fail_test "file did not pass validation: $file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo "The following tests (in: examples/lang/) have failed:"
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

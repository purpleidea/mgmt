#!/bin/bash
# check that our modules still build, even if we don't run them here

# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

failures=''

# Test modules/ directory to see if the .mcl files compile correctly.

find_mcl_modules() {
	git ls-files | grep '\.mcl$' | grep '^modules/' | grep -v 'examples/lang/'
}

# TODO: It might be better to only test from the root module entrypoint.
for file in $(find_mcl_modules); do
	#echo "mcl: $file"
	run-test ./mgmt run --tmp-prefix lang --only-unify "$file" &> /dev/null || fail_test "could not compile: $file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo "The following tests (in: ${linkto}) have failed:"
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

#!/bin/bash

. test/util.sh

echo "running test-gotest.sh $1"

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

base=$(go list .)
for pkg in `go list ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old/" | grep -v "^${base}/tmp/"`; do
	echo "Testing: $pkg"
	# FIXME: can we capture and output the stderr from these tests too?
	run-test go test "$pkg"
	if [ "$1" = "--race" ]; then
		run-test go test -race "$pkg"
	fi
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following `go test` runs have failed:'
	echo -e "$failures"
	exit 1
fi
echo 'PASS'

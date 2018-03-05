#!/bin/bash

echo running "$0" "$@"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

base=$(go list .)
for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old/" | grep -v "^${base}/tmp/" | grep -v "^${base}/integration/"`; do
	echo -e "\ttesting: $pkg"
	run-test go test "$pkg"
	if [ "$1" = "--race" ]; then
		shift
		run-test go test -race "$pkg"
	fi
done

if [[ "$@" = *"--integration"* ]]; then
	run-test go test github.com/purpleidea/mgmt/integration/
fi

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following `go test` runs have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

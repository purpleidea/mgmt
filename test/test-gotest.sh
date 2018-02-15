#!/bin/bash

echo "running test-gotest.sh $1"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

tmpdir="`$mktemp --tmpdir -d tmp.XXX`"	# get a dir outside of the main package
log="$tmpdir/$(basename $0 .sh).log"

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

base=$(go list .)
for pkg in `go list ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old/" | grep -v "^${base}/tmp/"`; do
	echo -e "\ttesting: $pkg"
	# FIXME: can we capture and output the stderr from these tests too?
	run-test go test "$pkg" > "$log"
	if [ "$1" = "--race" ]; then
		run-test go test -race "$pkg" > "$log"
	fi
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	cat "$log"
	echo 'The following `go test` runs have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

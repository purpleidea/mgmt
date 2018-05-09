#!/bin/bash

echo running "$0" "$@"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

# if we want to run this test as root, use build tag -root to ask each test...
XSUDO=''
XTAGS=''
if [[ "$@" = *"--root"* ]]; then
	if ! timeout 1s sudo -A true; then
		echo "sudo disabled: can't run as root"
		exit 1
	fi
	XSUDO='sudo -E'
	XTAGS='-tags root'
fi

failures=''
function run-test()
{
	$XSUDO $@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

base=$(go list .)
if [[ "$@" = *"--integration"* ]]; then
	if [[ "$@" = *"--race"* ]]; then
		GOCACHE=off run-test go test -race "${base}/integration" -v ${XTAGS}
	else
		GOCACHE=off run-test go test "${base}/integration" -v ${XTAGS}
	fi
else
	for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old" | grep -v "^${base}/old/" | grep -v "^${base}/tmp" | grep -v "^${base}/tmp/" | grep -v "^${base}/integration"`; do
		echo -e "\ttesting: $pkg"
		if [[ "$@" = *"--race"* ]]; then
			GOCACHE=off run-test go test -race "$pkg" ${XTAGS}
		else
			GOCACHE=off run-test go test "$pkg" ${XTAGS}
		fi
	done
fi

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following `go test` runs have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

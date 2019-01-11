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

# NOTE: you can run `go test` with the -tags flag to skip certain tests, eg:
# go test -tags nodocker github.com/purpleidea/mgmt/engine/resources -v
base=$(go list .)
if [[ "$@" = *"--integration"* ]]; then
	if [[ "$@" = *"--race"* ]]; then
		# adding -count=1 replaces the GOCACHE=off fix that was removed
		run-test go test -count=1 -race "${base}/integration" -v ${XTAGS}
	else
		run-test go test -count=1 "${base}/integration" -v ${XTAGS}
	fi
else
	for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old" | grep -v "^${base}/old/" | grep -v "^${base}/tmp" | grep -v "^${base}/tmp/" | grep -v "^${base}/integration"`; do
		echo -e "\ttesting: $pkg"
		if [[ "$@" = *"--race"* ]]; then
			run-test go test -count=1 -race "$pkg" ${XTAGS}
		else
			run-test go test -count=1 "$pkg" ${XTAGS}
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

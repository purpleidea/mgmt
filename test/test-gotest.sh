#!/bin/bash

echo running "$0" "$@"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

# travis is slow for some reason
if env | grep -q -e '^TRAVIS=true$'; then
	export GO_TEST_TIMEOUT_SCALE=3
fi

# if we want to run this test as root, use build tag -root to ask each test...
XSUDO=''
XTAGS=()
if [[ "$@" = *"--root"* ]]; then
	if ! timeout 1s sudo -A true; then
		echo "sudo disabled: can't run as root"
		exit 1
	fi
	XSUDO='sudo -E'
	XTAGS+=('root')
fi

# As per https://github.com/travis-ci/docs-travis-ci-com/blob/master/user/docker.md
# Docker is not supported on Travis macOS test instances.
if [[ "$TRAVIS_OS_NAME" == "osx" ]]; then
	XTAGS+=('nodocker')
fi

failures=''
function run-test()
{
	$XSUDO $@ -tags="${XTAGS[*]}" || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# NOTE: you can run `go test` with the -tags flag to skip certain tests, eg:
# go test -tags nodocker github.com/purpleidea/mgmt/engine/resources -v
base=$(go list .)
if [[ "$@" = *"--integration"* ]]; then
	if [[ "$@" = *"--race"* ]]; then
		# adding -count=1 replaces the GOCACHE=off fix that was removed
		run-test go test -count=1 -race "${base}/integration" -v
	else
		run-test go test -count=1 "${base}/integration" -v
	fi
else
	for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old" | grep -v "^${base}/old/" | grep -v "^${base}/tmp" | grep -v "^${base}/tmp/" | grep -v "^${base}/integration"`; do
		echo -e "\ttesting: $pkg"
		if [[ "$@" = *"--race"* ]]; then
			# split up long tests to avoid CI timeouts
			if [ "$pkg" = "${base}/lang" ]; then # pkg lang is big!
				for sub in `go test "${base}/lang" -list Test`; do
					if [ "$sub" = "ok" ]; then break; fi # skip go test output artifact
					echo -e "\t\tsub-testing: $sub"
					run-test go test -count=1 -race "$pkg" -run "$sub"
				done
			else
				run-test go test -count=1 -race "$pkg"
			fi
		else
			run-test go test -count=1 "$pkg"
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

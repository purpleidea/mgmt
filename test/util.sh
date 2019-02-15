#!/bin/bash

# common settings and functions for test scripts

# get the fully expanded path of the project directory
ROOT="$(realpath "$(dirname "$(realpath "${BASH_SOURCE}")")/..")"

# absolute location to freshly build binary to be used for testing
export MGMT="$ROOT/mgmt"

if [[ $(uname) == "Darwin" ]] ; then
	export timeout="gtimeout"
	export mktemp="gmktemp"
	export STAT="gstat"
else
	export timeout="timeout"
	export mktemp="mktemp"
	export STAT="stat"
fi

fail_test()
{
	echo -e "FAIL: $@"
	exit 1
}

function run-test()
{
	"$@" || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# travis expander helpers from:
# https://github.com/travis-ci/travis-rubies/blob/build/build.sh
fold_start() {
	if env | grep -q -e '^TRAVIS=true$'; then
		echo -e "travis_fold:start:$1\033[33;1m$2\033[0m"
	fi
}
fold_end() {
	if env | grep -q -e '^TRAVIS=true$'; then
		echo -e "\ntravis_fold:end:$1\r"
	fi
}

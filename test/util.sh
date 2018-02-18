#!/bin/bash

# common settings and functions for test scripts

# get the fully expanded path of the project directory
ROOT="$(realpath "$(dirname "$(realpath "${BASH_SOURCE}")")/..")"

# absolute location to freshly build binary to be used for testing
export MGMT="$ROOT/mgmt"

if [[ $(uname) == "Darwin" ]] ; then
	export timeout="gtimeout"
	export mktemp="gmktemp"
else
	export timeout="timeout"
	export mktemp="mktemp"
fi

fail_test()
{
	echo "FAIL: $@"
	exit 1
}

function run-test()
{
	"$@" || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

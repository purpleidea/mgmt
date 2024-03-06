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

TIMEOUT="$timeout --kill-after=360s --signal=QUIT 300s"

in_env() {
	if [ $# -eq 0 ]; then
		test -n "$CI" -o -n "$GITHUB_ACTION" -o -n "$TRAVIS" -o -n "$JENKINS_URL" -o -n "$DOCKER"
		return $?
	fi

	for var in "$@"; do
		case "$var" in
		github)
			test -n "$GITHUB_ACTION" && return 0;;
		travis)
			test "$TRAVIS" = "true" && return 0;;
		jenkins)
			test -n "$JENKINS_URL" && return 0;;
		docker)
			test -n "$DOCKER" && return 0;;
		*)
			continue;;
		esac
	done
	return 1
}

fail_test() {
	if in_env github; then
		echo "::error::$@"
	else
		echo -e "FAIL: $@"
	fi
	exit 1
}

function run-test() {
	"$@" || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# travis expander helpers from:
# https://github.com/travis-ci/travis-rubies/blob/build/build.sh
fold_start() {
	if in_env travis; then
		echo -e "travis_fold:start:$1\033[33;1m${@:2}\033[0m"
	elif in_env github; then
		echo "::group::$@"
	fi
}
fold_end() {
	if in_env travis; then
		echo -e "\ntravis_fold:end:$1\r"
	elif in_env github; then
		echo "::endgroup::"
	fi
}

assert_sudo() {
	if ! timeout 1s sudo -A true; then
		echo "sudo disabled: $@"
		exit
	fi
}

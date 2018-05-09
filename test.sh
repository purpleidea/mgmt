#!/bin/bash -e
# runs all (or selected) test suite(s) in test/ and aggregates results
# Usage:
#	./test.sh
#	./test.sh gofmt

# library of utility functions
# shellcheck disable=SC1091
. test/util.sh

# allow specifying a single testsuite to run
testsuite="$1"

# print environment when running all testsuites
test -z "$testsuite" && (echo "ENV:"; env; echo; )

# run a test and record failures
function run-testsuite()
{
	testname="$(basename "$1" .sh)"
	# if not running all tests or if this test is not explicitly selected, skip it
	if test -z "$testsuite" || test "test-$testsuite" = "$testname";then
		$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
	fi
}

# only run test if it is explicitly selected, otherwise report it is skipped
function skip-testsuite()
{
	testname=$(basename "$1" .sh)
	# show skip message only when running full suite
	if test -z "$testsuite";then
		echo skipping "$@" "($REASON)"
		echo 'SKIP'
	else
		# if a skipped suite is explicity called, run it anyway
		if test "test-$testsuite" == "$testname";then
			run-testsuite "$@"
		fi
	fi
}

# used at the end to tell if everything went fine
failures=''

run-testsuite ./test/test-misc.sh
run-testsuite ./test/test-gofmt.sh
run-testsuite ./test/test-yamlfmt.sh
run-testsuite ./test/test-bashfmt.sh
run-testsuite ./test/test-headerfmt.sh
run-testsuite ./test/test-markdownlint.sh
run-testsuite ./test/test-commit-message.sh
run-testsuite ./test/test-govet.sh
run-testsuite ./test/test-examples.sh
run-testsuite ./test/test-gotest.sh

# skipping: https://github.com/purpleidea/mgmt/issues/327
# run-test ./test/test-crossbuild.sh

# do these longer tests only when running on ci
if env | grep -q -e '^TRAVIS=true$' -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	run-testsuite ./test/test-shell.sh
	run-testsuite ./test/test-gotest.sh --race
	run-testsuite ./test/test-integration.sh
	run-testsuite ./test/test-integration.sh --race

	# XXX: fix and enable these on travis (sudo: go: command not found)
	#run-testsuite ./test/test-gotest.sh --root
	#run-testsuite ./test/test-gotest.sh --root --race
	#run-testsuite ./test/test-integration.sh --root
	#run-testsuite ./test/test-integration.sh --root --race
else
	REASON="CI server only test" skip-testsuite ./test/test-shell.sh
	REASON="CI server only test" skip-testsuite ./test/test-gotest.sh --race
	REASON="CI server only test" skip-testsuite ./test/test-integration.sh
	REASON="CI server only test" skip-testsuite ./test/test-integration.sh --race

	REASON="CI server only test" skip-testsuite ./test/test-gotest.sh --root
	REASON="CI server only test" skip-testsuite ./test/test-gotest.sh --root --race
	REASON="CI server only test" skip-testsuite ./test/test-integration.sh --root
	REASON="CI server only test" skip-testsuite ./test/test-integration.sh --root --race
fi

run-testsuite ./test/test-gometalinter.sh

# FIXME: this now fails everywhere :(
skip-testsuite ./test/test-reproducible.sh

# run omv tests on jenkins physical hosts only
if env | grep -q -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	run-testsuite ./test/test-omv.sh
else
	REASON="CI server only test" skip-testsuite ./test/test-omv.sh
fi

REASON="https://github.com/purpleidea/mgmt/issues/327" skip-testsuite ./test/test-crossbuild.sh

run-testsuite ./test/test-golint.sh	# test last, because this test is somewhat arbitrary

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	echo 'You can rerun a single suite like so:'
	echo
	echo '`make test-gofmt` or `make test-shell-<testname>`'
	exit 1
fi
echo 'ALL PASSED'

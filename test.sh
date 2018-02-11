#!/bin/bash -e
# test suite...
echo running test.sh
echo "ENV:"
env

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# ensure there is no trailing whitespace or other whitespace errors
run-test git diff-tree --check $(git hash-object -t tree /dev/null) HEAD

# ensure entries to authors file are sorted
start=$(($(grep -n '^[[:space:]]*$' AUTHORS | awk -F ':' '{print $1}' | head -1) + 1))
run-test diff <(tail -n +$start AUTHORS | sort) <(tail -n +$start AUTHORS)

run-test ./test/test-gofmt.sh
run-test ./test/test-yamlfmt.sh
run-test ./test/test-bashfmt.sh
run-test ./test/test-headerfmt.sh
run-test ./test/test-commit-message.sh
run-test ./test/test-govet.sh
run-test ./test/test-examples.sh
run-test ./test/test-gotest.sh

# skipping: https://github.com/purpleidea/mgmt/issues/327
# run-test ./test/test-crossbuild.sh

# do these longer tests only when running on ci
if env | grep -q -e '^TRAVIS=true$' -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	run-test ./test/test-shell.sh
	#run-test ./test/test-gotest.sh --race	# XXX: temporarily disabled...
fi

run-test ./test/test-gometalinter.sh
# FIXME: this now fails everywhere :(
#run-test ./test/test-reproducible.sh

# run omv tests on jenkins physical hosts only
if env | grep -q -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	run-test ./test/test-omv.sh
fi
run-test ./test/test-golint.sh	# test last, because this test is somewhat arbitrary

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	exit 1
fi
echo 'ALL PASSED'

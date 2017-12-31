#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# test suite...
# Invoke as "./test.sh" from the root of the git repo.
################################################################################

info "Environment variables:"
indent "$(env)"

smitty go version

info "Go environment:"
indent "$(go env)"

# Install dependencies that do not need root.
misc/bootstrap.sh

# Fail if any dependencies are missing.
misc/validate-dependencies.sh

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

# do these longer tests only when running on ci
if env | grep -q -e '^TRAVIS=true$' -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	run-test ./test/test-shell.sh
	run-test ./test/test-gotest.sh --race
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
	err 'The following tests have failed:'
	indent "$failures"
	exit 1
fi

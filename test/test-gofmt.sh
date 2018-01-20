#!/bin/bash
# original version of this script from kubernetes project, under ALv2 license

echo running test-gofmt.sh
set -o errexit
set -o nounset
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

#GO_VERSION=($(go version))
#
#if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.2|go1.3|go1.4|go1.5|go1.6|go1.7|go1.8|go1.9|devel') ]]; then
#	fail_test "Unknown go version '${GO_VERSION[2]}', failing gofmt."
#fi

find_files() {
	git ls-files | grep '\.go$'
}

# gofmt -s -l
GOFMT="gofmt"
bad_files=$(find_files | xargs $GOFMT -s -l)
if [[ -n "${bad_files}" ]]; then
	fail_test "The following golang files are not properly formatted (gofmt -s -l): ${bad_files}"
fi
#goimports -l
GOFMT="goimports"
bad_files=$(find_files | xargs $GOFMT -l)
if [[ -n "${bad_files}" ]]; then
	fail_test "The following golang files are not properly formatted (goimports -l): ${bad_files}"
fi
echo 'PASS'

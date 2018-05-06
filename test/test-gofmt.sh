#!/bin/bash
# original version of this script from kubernetes project, under ALv2 license

echo running "$0"
set -o errexit
set -o nounset
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

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

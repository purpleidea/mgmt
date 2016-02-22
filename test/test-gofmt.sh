#!/bin/bash
# original version of this script from kubernetes project, under ALv2 license
echo running test-gofmt.sh
set -o errexit
set -o nounset
set -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")/..

GO_VERSION=($(go version))

if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.2|go1.3|go1.4|go1.5|go1.6') ]]; then
	echo "Unknown go version '${GO_VERSION}', skipping gofmt."
	exit 0
fi

cd "${ROOT}"

find_files() {
	git ls-files | grep '\.go$'
}

GOFMT="gofmt"	# we prefer to not use the -s flag, which is pretty annoying...
bad_files=$(find_files | xargs $GOFMT -l)
if [[ -n "${bad_files}" ]]; then
	echo 'FAIL'
	echo 'The following golang files are not properly formatted:'
	echo "${bad_files}"
	exit 1
fi

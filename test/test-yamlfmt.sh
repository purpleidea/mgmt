#!/bin/bash
# check for any yaml files that aren't properly formatted

echo running "$0"
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

LINTER="pipenv run yamllint"
if ! $LINTER -h >/dev/null ; then
	fail_test "The 'yamllint' utility can't be found."
fi

find_files() {
	git ls-files | grep '\.ya\?ml$' \
		|| fail_test "Could not find yaml files via git ls-files"
}

bad_files=$(
	find_files | xargs $LINTER | grep '\.ya\?ml$'
)

if [[ -n "${bad_files}" ]]; then
	fail_test "The following yaml files are not properly formatted: ${bad_files}"
fi
echo 'PASS'

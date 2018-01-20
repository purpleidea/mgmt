#!/bin/bash
# check for any bash files that aren't properly formatted
# TODO: this is hardly exhaustive

echo running test-bashfmt.sh
set -o errexit
set -o nounset
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

find_files() {
	git ls-files | grep -e '\.sh$' -e '\.bash$' | grep -v 'misc/delta-cpu.sh'
}

bad_files=$(
	for i in $(find_files); do
		# search for more than one leading space, to ensure we use tabs
		if grep -q '^  ' "$i"; then
			echo "$i"
		fi
	done
)

if [[ -n "${bad_files}" ]]; then
	fail_test "The following bash files are not properly formatted: ${bad_files}"
fi
echo 'PASS'

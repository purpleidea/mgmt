#!/usr/bin/env bash
# check that comments only contain standard ASCII characters

# library of utility functions
# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}" || exit 1

# list of forbidden characters
FORBIDDEN='[‘’“”←→–—…±]'

# exclude files that are expected to contain non-ASCII
find_files() {
	repo_files | grep -vE '^(lang/core/generated_funcs.go|AUTHORS|THANKS|go.sum|data/locales/.*\.po)$'
}

bad_files=$(
	for i in $(find_files); do
		# only check text files (grep -I skips binary)
		# look for lines starting with // or # that contain forbidden characters
		if grep -rnIE "([#]|//).*$FORBIDDEN" "$i"; then
			echo "$i"
		fi
	done
)

if [[ -n "${bad_files}" ]]; then
	fail_test "The following files contain forbidden characters in comments:\n${bad_files}"
fi
echo 'PASS'

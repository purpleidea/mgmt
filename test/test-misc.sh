#!/bin/bash
# simple tests that don't deserve their own testfile

# library of utility functions
# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}" || exit 1

failures=''

# ensure there is no trailing whitespace or other whitespace errors
run-test git diff-tree --check $(git hash-object -t tree /dev/null) HEAD

# ensure entries to authors file are sorted
start=$(($(grep -n '^[[:space:]]*$' AUTHORS | awk -F ':' '{print $1}' | head -1) + 1))
run-test diff <(tail -n +$start AUTHORS | sort) <(tail -n +$start AUTHORS)

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo "The following tests have failed:"
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

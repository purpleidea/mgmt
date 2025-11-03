#!/usr/bin/env bash
# check that gettext files are up to date

echo running "$0"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

function gettext-strings() {
	make gettext # this will cause git edits to data/locales/ if needed...

	git diff-index --quiet HEAD # false if there are changes
	ret=$?
	if [ $ret -ne 0 ]; then
		git diff # tell us why
		return 1
	fi
	return 0
}

run-test gettext-strings "$file"

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

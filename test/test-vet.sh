#!/bin/bash
# vet a few random things

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

function parser-indentation() {
	# the $ before the \t magically makes grep match the tab somehow...
	if grep $'\t|' "$1"; then	# indent the pipe too
		return 1
	fi
	# indent the pipe too (no spaces!)
	# skip over any lines with leading comments
	# or boolean or '||'
	# the $ before the \t magically makes grep match the tab somehow...
	if grep ' |' "$1" | grep -v '^['$'\t'']*// ' | grep -v '||'; then
		return 1
	fi
	if grep '^ ' "$1"; then	# format with tabs, no leading spaces
		return 1
	fi

	return 0
}

function parser-conflicts() {
	# in the test, run goyacc, but don't leave any output files around
	if goyacc -o /dev/null -v /dev/null "$1" | grep 'conflict'; then	# grammar is ambiguous
		return 1
	fi

	return 0
}

# loop through individual *.y files
for file in `find . -maxdepth 9 -type f -name '*.y' -not -path './old/*' -not -path './tmp/*' -not -path './vendor/*'`; do
	run-test parser-indentation "$file"
	run-test parser-conflicts "$file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

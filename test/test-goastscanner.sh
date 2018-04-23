#!/bin/bash

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

exclude=()
#exclude+=('G101')				# example on how to use exclude
#exclude+=('G203')				# example on how to use exclude
#exclude+=('G401')				# example on how to use exclude

excludes=$(IFS=,; echo "${exclude[*]}")		# join to comma separated string

gas='gas'
if [ "${excludes}" != "" ]; then
	gas="$gas -exclude=${excludes}"
fi

# if we need to exclude some generated files:
# TODO: https://github.com/GoASTScanner/gas/issues/172
#gas="$gas -skip=lang/lexer.nn.go"
#gas="$gas -skip=lang/y.go"
#gas="$gas -skip=bindata/bindata.go"

final="$gas ./..."	# final
echo "Using: $final"

run-test $final || fail_test "go ast scanner did not pass"

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

#!/bin/bash
# check that go vet passes

. test/util.sh

echo running test-govet.sh

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}"

function simplify-gocase() {
	if grep 'case _ = <-' "$1"; then
		return 1	# 'case _ = <- can be simplified to: case <-'
	fi
	return 0
}

function token-coloncheck() {
	# add quotes to avoid matching three X's
	if grep -Ei "[\/]+[\/]+[ ]*+(FIXME[^:]|TODO[^:]|X"'X'"X[^:])" "$1"; then
		return 1	# tokens must end with a colon
	fi
	return 0
}

for file in `find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*'`; do
	run-test go vet "$file" || fail_test "go vet did not pass"	# since it doesn't output an ok message on pass
	run-test grep 'log.' "$file" | grep '\\n"' && fail_test 'no newline needed in log.Printf()'	# no \n needed in log.Printf()
	run-test simplify-gocase "$file"
	run-test token-coloncheck "$file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	exit 1
fi
echo 'PASS'

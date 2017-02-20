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
	grep 'case _ = <-' "$1" && fail_test 'case _ = <- can be simplified to: case <-'	# this can be simplified
}

function token-coloncheck() {
	grep -Ei "[\/]+[\/]+[ ]*+(FIXME[^:]|TODO[^:]|XXX[^:])" "$1" && fail_test 'Token is missing a colon'	# tokens must end with a colon
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

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

GO_VERSION=($(go version))

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
	if [[ -z $(echo "${GO_VERSION[2]}" | grep -E 'go1.2|go1.3|go1.4|go1.5|go1.6|go1.7|go1.8') ]]; then
		# workaround go vet issues by adding the new -source flag (go1.9+)
		run-test go vet -source "$file" || fail_test "go vet -source did not pass"
	else
		run-test go vet "$file" || fail_test "go vet did not pass"	# since it doesn't output an ok message on pass
	fi
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

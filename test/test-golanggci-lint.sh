#!/bin/bash
# check a bunch of linters with golangci-lint
# TODO: run this from the test-golint.sh file instead to check for deltas

echo running "$0"

# ensure golangci-lint is available
command -v golangci-lint >/dev/null 2>&1 || { echo >&2 "golangci-lint not found"; exit 1; }

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# TODO: run more linters here if we're brave...
golangci_lint='golangci-lint run --disable-all'
golangci_lint="$golangci_lint --enable-all"
golangci_lint="$golangci_lint --skip-dirs=old,tmp,vendor,examples,test"
# exclude generated files:
# TODO: at least until https://github.com/golangci/golangci-lint/issues/192
golangci_lint="$golangci_lint --skip-files=lang/parser/lexer.nn.go,lang/parser/y.go,lang/types/kind_stringer.go,lang/interpolate/parse.generated.go"

golangci_lint_cmd="$golangci_lint"	# final
echo "Using: $golangci_lint_cmd"

# loop through directories in an attempt to scan each go package
# TODO: lint the *.go examples as individual files and not as a single *.go
for dir in `find * -maxdepth 9 -type d -not -path 'old/*' -not -path 'old' -not -path 'tmp/*' -not -path 'tmp' -not -path 'vendor/*' -not -path 'examples/*' -not -path 'test/*'`; do
	#echo "Running in: $dir"

	match="$dir/*.go"
	#echo "match is: $match"
	if ! ls $match &>/dev/null; then
		#echo "skipping: $match"
		continue	# no *.go files found
	fi

	#echo "Running: $golangci_lint_cmd '$dir'"
	run-test $golangci_lint_cmd "$dir" || fail_test "golangci-lint did not pass"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'


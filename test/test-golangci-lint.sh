#!/usr/bin/env bash
# check a bunch of linters with the golangci_lint
# TODO: run this from the test-golint.sh file instead to check for deltas

echo running "$0"

# ensure golangci_lint is available
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
glc='golangci-lint run --disable-all'

# enable linters here
glc="$glc --enable=goimports"
glc="$glc --enable=revive"
glc="$glc --enable=misspell"

# exclude files and directories here:
# glc = "$glc --skip-files=EnterFileName"
# glc = "$glc --skip-dirs=EnterDirName"

golangci_lint="$glc"	# final
echo "Using: $golangci_lint"

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

	#echo "Running: $golangci_lint '$dir'"
	run-test $golangci_lint "$dir" || fail_test "golangci_lint did not pass"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

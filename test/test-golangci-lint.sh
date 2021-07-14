#!/bin/bash

# print current test
echo running "$0"

command -v golangci-lint >/dev/null 2>&1 || { echo >&2 "golangci-lint not found"; exit 1; }

# configure settings for test scripts
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}" # Enter mgmt root
. test/util.sh

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# using .golangci.yml config file settings in ROOT
gcl='golangci-lint run' 

# commented out from gometalinter linter check
# gcl='golangci-lint run --enable aligncheck'
# gcl='golangci-lint run --enable dupl'
# gcl='golangci-lint run --enable errcheck'
# gcl='golangci-lint run --enable gas'
# gcl='golangci-lint run --enable goconst'
# gcl='golangci-lint run --enable gocyclo'
# gcl='golangci-lint run --enable gotype'
# gcl='golangci-lint run --enable unconvert'

# TODO: only a few fixes needed
# gcl='golangci-lint run --enable deadcode'
# gcl='golangci-lint run --enable gosimple'
# gcl='golangci-lint run --enable ineffassign'
# gcl='golangci-lint run --enable interfacer'
# gcl='golangci-lint run --enable lll --line-length=200'
# gcl='golangci-lint run --enable safesql'
# gcl='golangci-lint run --enable staticcheck'
# gcl='golangci-lint run --enable structcheck'
# gcl='golangci-lint run --enable unparam'
# gcl='golangci-lint run --enable unused'
# gcl='golangci-lint run --enable varcheck'

for dir in `find * -maxdepth 9 -type d -not -path 'old/*' -not -path 'old' -not -path 'tmp/*' -not -path 'tmp' -not -path 'vendor/*' -not -path 'examples/*' -not -path 'test/*' -not -path 'interpolate/*'`; do
	
    # doesn't acquire files individually, but treats them as a set of * files
    match="$dir/*.go"

	if ! ls $match &>/dev/null; 
    then
		continue	# no *.go files found
    fi

    run-test $gcl "$dir" || fail_test "golangci-lint did not pass"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'
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

# commented out from gometalinter linter test
# aligncheck, dupl, errcheck, gas, goconst, gocyclo, gotype, unconvert

# TODO: only a few fixes needed
# linters: deadcode, gosimple, ineffassign, interfacer, lll --line-length=200
# safesql, staticcheck, structcheck, unparam, unused, varcheck

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
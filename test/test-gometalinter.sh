#!/bin/bash
# check a bunch of linters with the gometalinter
# TODO: run this from the test-golint.sh file instead to check for deltas

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

# TODO: run more linters here if we're brave...
gml='gometalinter --disable-all'
#gml="$gml --enable=aligncheck"
#gml="$gml --enable=deadcode"			# TODO: only a few fixes needed
#gml="$gml --enable=dupl"
#gml="$gml --enable=errcheck"
#gml="$gml --enable=gas"
#gml="$gml --enable=goconst"
#gml="$gml --enable=gocyclo"
gml="$gml --enable=goimports"
gml="$gml --enable=golint"
#gml="$gml --enable=gosimple"			# TODO: only a few fixes needed
#gml="$gml --enable=gotype"
#gml="$gml --enable=ineffassign"		# TODO: only a few fixes needed
#gml="$gml --enable=interfacer"			# TODO: only a few fixes needed
#gml="$gml --enable=lll --line-length=200"	# TODO: only a few fixes needed
gml="$gml --enable=misspell"
#gml="$gml --enable=safesql"			# FIXME: made my machine slow
#gml="$gml --enable=staticcheck"		# TODO: only a few fixes needed
#gml="$gml --enable=structcheck"		# TODO: only a few fixes needed
#gml="$gml --enable=unconvert"
#gml="$gml --enable=unparam"			# TODO: only a few fixes needed
#gml="$gml --enable=unused"			# TODO: only a few fixes needed
#gml="$gml --enable=varcheck"			# TODO: only a few fixes needed
gometalinter="$gml"

echo "Using: $gometalinter"
# loop through directories in an attempt to scan each go package
# TODO: lint the *.go examples as individual files and not as a single *.go
for dir in `find . -maxdepth 5 -type d -not -path './old/*' -not -path './old' -not -path './tmp/*' -not -path './tmp' -not -path './.*' -not -path './vendor/*' -not -path './bindata/*' -not -path ' ./examples/*' -not -path './test/*'`; do
	#echo "Running in: $dir"

	match="$dir/*.go"
	#echo "match is: $match"
	if ! ls $match &>/dev/null; then
		#echo "skipping: $match"
		continue	# no *.go files found
	fi

	#echo "Running: $gometalinter '$dir'"
	run-test $gometalinter "$dir" || fail_test "gometalinter did not pass"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

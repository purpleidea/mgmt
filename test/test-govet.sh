#!/bin/bash
# check that go vet passes

echo running "$0"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

make --quiet -C test	# run make in test directory to prepare any needed tools

failures=''
function run-test()
{
	$@ || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

GO_VERSION=($(go version))

function typos() {
	if grep -i 'reversable' "$1"; then	# the word is "reversible"
		return 1
	fi
	return 0
}

function simplify-gocase() {
	if grep 'case _ = <-' "$1"; then
		return 1	# 'case _ = <- can be simplified to: case <-'
	fi
	return 0
}

function token-coloncheck() {
	# add quotes to avoid matching three X's
	if grep -Ei "[\/]+[\/]+[ ]*(T"'O''D'"O[^:]|F"'I''X''M'"E[^:]|X"'X'"X[^:])" "$1"; then
		return 1	# tokens must end with a colon
	fi
	# tokens must be upper case
	if grep -E "[t][Oo][Dd][Oo]|[Tt][o][Dd][Oo]|[Tt][Oo][d][Oo]|[Tt][Oo][Dd][o]|[f][Ii][Xx][Mm][Ee]|[Ff][i][Xx][Mm][Ee]|[Ff][Ii][x][Mm][Ee]|[Ff][Ii][Xx][m][Ee]|[Ff][Ii][Xx][Mm][e]|[x][Xx][Xx]|[Xx][x][Xx]|[Xx][Xx][x]" "$1"; then
		return 1
	fi
	return 0
}

function naked-error() {
	# the $ before the \t magically makes grep match the tab somehow...
	if grep $'\terrors.New(' "$1"; then	# missing a leading return
		return 1
	fi
	if grep $'\tfmt.Errorf(' "$1"; then	# missing a leading return
		return 1
	fi
	if grep $'\terrwrap.Wrap' "$1"; then	# missing a leading return
		return 1
	fi
	return 0
}

# catch errors that start with a capital
function lowercase-errors() {
	if [[ $1 == *"/bindata.go" ]]; then	# ends with bindata.go ?
		return 0	# skip those generated files
	fi

	if grep -E 'errors\.New\(\"[A-Z]' "$1"; then
		return 1
	fi
	if grep -E 't\.Errorf\(\"[A-Z]' "$1"; then # t.Errorf or fmt.Errorf
		return 1
	fi
	# TODO: add errwrap.Wrap* related matching
	return 0
}

function consistent-imports() {
	if [ "$1" = './util/errwrap/errwrap.go' ]; then
		return 0
	fi

	# import as github.com/purpleidea/mgmt/util/errwrap
	if grep $'\t"github.com/pkg/errors"' "$1"; then
		return 1
	fi
	# import as github.com/purpleidea/mgmt/util/errwrap
	if grep $'\t"github.com/hashicorp/go-multierror"' "$1"; then
		return 1
	fi
	# import as langutil
	if grep $'\t"github.com/purpleidea/mgmt/lang/util"' "$1"; then
		return 1
	fi
	# import as engineutil
	if grep $'\t"github.com/purpleidea/mgmt/engine/util"' "$1"; then
		return 1
	fi
	if grep '"golang.org/x/net/context"' "$1"; then	# use built-in context
		return 1
	fi
}

function reflowed-comments() {
	if [[ $1 == *"/bindata.go" ]]; then	# ends with bindata.go ?
		return 0	# skip those generated files
	fi

	if [ "$1" = './lang/funcs/core/generated_funcs.go' ]; then
		return 0
	fi

	if [ "$1" = './lang/lexer.nn.go' ]; then
		return 0
	fi

	if [ "$1" = './lang/interpolate/parse.generated.go' ]; then
		return 0
	fi

	./test/comment_parser "$1"
}

# run go vet on a per-package basis
base=$(go list .)
for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old" | grep -v "^${base}/old/" | grep -v "^${base}/tmp" | grep -v "^${base}/tmp/"`; do
	echo -e "\tgo vet: $pkg"
	run-test go vet -source "$pkg" || fail_test "go vet -source did not pass pkg"

done

# loop through individual *.go files
for file in `find . -maxdepth 9 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*' -not -path './vendor/*'`; do
	#if [[ $file == "./vendor/"* ]]; then # skip files that start with...
	#	continue
	#fi
	run-test grep 'log.Print' "$file" | grep '\\n"' && fail_test 'no newline needed in log.Print*()'	# no \n needed in log.Printf or log.Println
	run-test typos "$file"
	run-test simplify-gocase "$file"
	run-test token-coloncheck "$file"
	run-test naked-error "$file"
	run-test lowercase-errors "$file"
	run-test consistent-imports "$file"
	run-test reflowed-comments "$file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

#!/bin/bash
# check that go vet passes

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

GO_VERSION=($(go version))

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

function consistent-imports() {
	if [ "$1" = './util/errwrap/errwrap.go' ]; then
		return 0
	fi

	if grep $'\t"github.com/pkg/errors"' "$1"; then	# import as errwrap
		return 1
	fi
	if grep $'\t"github.com/hashicorp/go-multierror"' "$1"; then	# import as multierr
		return 1
	fi
	if grep $'\t"github.com/purpleidea/mgmt/engine/util"' "$1"; then	# import as engineUtil
		return 1
	fi
}

# run go vet on a per-package basis
base=$(go list .)
for pkg in `go list -e ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old" | grep -v "^${base}/old/" | grep -v "^${base}/tmp" | grep -v "^${base}/tmp/"`; do
	echo -e "\tgo vet: $pkg"
	# workaround go vet issues by adding the new -source flag (go1.9+)
	run-test go vet -source "$pkg" || fail_test "go vet -source did not pass pkg"

done

# loop through individual *.go files
for file in `find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*'`; do
	run-test grep 'log.Print' "$file" | grep '\\n"' && fail_test 'no newline needed in log.Print*()'	# no \n needed in log.Printf or log.Println
	run-test simplify-gocase "$file"
	run-test token-coloncheck "$file"
	run-test naked-error "$file"
	run-test consistent-imports "$file"
done

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

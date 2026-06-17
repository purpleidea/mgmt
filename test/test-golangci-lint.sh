#!/usr/bin/env bash
# check a bunch of linters with the golangci-lint
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

glc_config=$(mktemp --suffix=.yaml)
trap 'rm -f "$glc_config"' EXIT
# The yaml config requires space indentation, but our bash tester wants tabs.
# Indent with tabs below and use sed to swap leading tabs back to two spaces.
sed ':a; s/^\( *\)	/\1  /; ta' >"$glc_config" <<'EOF'
version: "2"
run:
	relative-path-mode: gomod
issues:
	# show every finding: the defaults (50 per linter, 3 per same issue)
	# silently truncate the output and hide the true number of problems.
	max-issues-per-linter: 0
	max-same-issues: 0
linters:
	settings:
		gosec:
			# G204 (subprocess with variable) and G304 (file inclusion
			# via variable) fire on nearly every file and exec operation
			# in mgmt: as a config management engine, running commands
			# and reading/writing files at config-supplied paths is its
			# core purpose, so these are intentional everywhere.
			excludes:
				- G204
				- G304
	exclusions:
		rules:
			- path: ^etcd/client/resources/resources\.go$
				text: '`thn` is a misspelling of `then`'
				source: '^[[:space:]]*(thn (:=|= append)|out, err := client\.Txn\(ctx, ifs, thn, els\))'
				linters:
					- misspell
			- path: ^lang/core/generated_funcs\.go$
				linters:
					- gosec
EOF

# TODO: run more linters here if we're brave...
glc="golangci-lint run --config=$glc_config --default=none"
glc_fmt='golangci-lint fmt --diff'

# enable linters here
glc="$glc --enable=arangolint"
glc="$glc --enable=asasalint"
glc="$glc --enable=asciicheck"
glc="$glc --enable=bidichk"
glc="$glc --enable=bodyclose"
glc="$glc --enable=canonicalheader"
#glc="$glc --enable=containedctx"
#glc="$glc --enable=contextcheck"
glc="$glc --enable=copyloopvar"
#glc="$glc --enable=cyclop"
glc="$glc --enable=decorder"
#glc="$glc --enable=depguard"
glc="$glc --enable=dogsled"
#glc="$glc --enable=dupl"
#glc="$glc --enable=dupword"
glc="$glc --enable=durationcheck"
#glc="$glc --enable=embeddedstructfieldcheck"
#glc="$glc --enable=err113"
#glc="$glc --enable=errcheck"
glc="$glc --enable=errchkjson"
#glc="$glc --enable=errname"
#glc="$glc --enable=errorlint"
#glc="$glc --enable=exhaustive"
#glc="$glc --enable=exhaustruct"
glc="$glc --enable=exptostd"
#glc="$glc --enable=fatcontext"
#glc="$glc --enable=forbidigo"
#glc="$glc --enable=forcetypeassert"
#glc="$glc --enable=funcorder"
#glc="$glc --enable=funlen"
glc="$glc --enable=ginkgolinter"
glc="$glc --enable=gocheckcompilerdirectives"
#glc="$glc --enable=gochecknoglobals"
#glc="$glc --enable=gochecknoinits"
glc="$glc --enable=gochecksumtype"
#glc="$glc --enable=gocognit"
#glc="$glc --enable=goconst"
#glc="$glc --enable=gocritic"
#glc="$glc --enable=gocyclo"
#glc="$glc --enable=godoclint"
#glc="$glc --enable=godot"
#glc="$glc --enable=godox"
glc="$glc --enable=goheader"
#glc="$glc --enable=gomoddirectives"
#glc="$glc --enable=gomodguard" # deprecated
#glc="$glc --enable=gomodguard_v2" # future
glc="$glc --enable=goprintffuncname"
glc="$glc --enable=gosec"
glc="$glc --enable=gosmopolitan"
glc="$glc --enable=govet"
glc="$glc --enable=grouper"
#glc="$glc --enable=iface"
glc="$glc --enable=importas"
#glc="$glc --enable=inamedparam"
#glc="$glc --enable=ineffassign"
#glc="$glc --enable=interfacebloat"
#glc="$glc --enable=intrange"
glc="$glc --enable=iotamixing"
#glc="$glc --enable=ireturn"
#glc="$glc --enable=lll"
glc="$glc --enable=loggercheck"
#glc="$glc --enable=maintidx"
glc="$glc --enable=makezero"
glc="$glc --enable=mirror"
glc="$glc --enable=misspell"
#glc="$glc --enable=mnd"
#glc="$glc --enable=modernize"
#glc="$glc --enable=musttag"
#glc="$glc --enable=nakedret"
#glc="$glc --enable=nestif"
#glc="$glc --enable=nilerr"
glc="$glc --enable=nilnesserr"
#glc="$glc --enable=nilnil"
#glc="$glc --enable=nlreturn"
#glc="$glc --enable=noctx"
#glc="$glc --enable=noinlineerr"
glc="$glc --enable=nolintlint"
#glc="$glc --enable=nonamedreturns"
glc="$glc --enable=nosprintfhostport"
#glc="$glc --enable=paralleltest"
#glc="$glc --enable=perfsprint"
#glc="$glc --enable=prealloc"
#glc="$glc --enable=predeclared"
glc="$glc --enable=promlinter"
glc="$glc --enable=protogetter"
glc="$glc --enable=reassign"
glc="$glc --enable=recvcheck"
#glc="$glc --enable=revive"
glc="$glc --enable=rowserrcheck"
glc="$glc --enable=sloglint"
glc="$glc --enable=spancheck"
glc="$glc --enable=sqlclosecheck"
#glc="$glc --enable=staticcheck"
#glc="$glc --enable=tagalign"
#glc="$glc --enable=tagliatelle"
glc="$glc --enable=testableexamples"
glc="$glc --enable=testifylint"
#glc="$glc --enable=testpackage"
#glc="$glc --enable=thelper"
glc="$glc --enable=tparallel"
glc="$glc --enable=unconvert"
#glc="$glc --enable=unparam"
glc="$glc --enable=unqueryvet"
#glc="$glc --enable=unused"
glc="$glc --enable=usestdlibvars"
#glc="$glc --enable=usetesting"
#glc="$glc --enable=varnamelen"
#glc="$glc --enable=wastedassign"
#glc="$glc --enable=whitespace"
#glc="$glc --enable=wrapcheck"
#glc="$glc --enable=wsl"
#glc="$glc --enable=wsl_v5"
glc="$glc --enable=zerologlint"

# enable formatters here
glc_fmt="$glc_fmt --enable=goimports"

echo "Using: $glc"
echo "Using: $glc_fmt"

root=$(pwd)
package_dirs=$(go list -e -f '{{.Dir}}' ./...) || fail_test "could not list golang packages"
packages=()
files=()
shopt -s nullglob
while IFS= read -r dir; do
	if [[ "$dir" == "$root" ]]; then
		path=.
	else
		path=./${dir#"$root"/}
	fi

	# exclude
	case "$path" in
		./examples|./examples/*) continue ;;
		./old|./old/*) continue ;;
		./tmp|./tmp/*) continue ;;
		./test|./test/*) continue ;;
		./engine/resources/http_server_ui|./engine/resources/http_server_ui/*) continue ;;
	esac

	packages+=("$path")
	files+=("$path"/*.go)
done <<< "$package_dirs"
shopt -u nullglob

run-test $glc "${packages[@]}" || fail_test "golangci-lint run did not pass"
run-test $glc_fmt "${files[@]}" || fail_test "golangci-lint fmt did not pass"

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo 'The following tests have failed:'
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

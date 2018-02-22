#!/bin/bash
# check for any markdown files that aren't in an ideal format

echo running "$0 $@"
set -o errexit
set -o nounset
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

MDL=`command -v mdl 2>/dev/null`
if [ -z $MDL ]; then
	fail_test "The 'mdl' utility can't be found."
fi

STYLE=$($mktemp)
# styles that we ignore... if they're too onerous, we can exclude them here...
cat << 'EOF' > $STYLE
all
exclude_rule 'MD010'	# Hard tabs
exclude_rule 'MD032'	# Lists should be surrounded by blank lines
exclude_rule 'MD013'	# Line length
exclude_rule 'MD040'	# Fenced code blocks should have a language specified
exclude_rule 'MD026'	# Trailing punctuation in header
exclude_rule 'MD024'	# Multiple headers with the same content
exclude_rule 'MD002'	# First header should be a top level header
exclude_rule 'MD041'	# First line in file should be a top level header
exclude_rule 'MD007'	# Unordered list indentation

# FIXME: no idea why this issue occurs
exclude_rule 'MD029'	# Ordered list item prefix

# FIXME: bug: https://github.com/markdownlint/markdownlint/issues/182
exclude_rule 'MD039'	# Spaces inside link text
EOF

#STYLE="test/mdl.style"	# style file

find_files() {
	git ls-files | grep '\.md$'
}

F=${1:-}	# only check this file from $1 is specified

bad_files=$(
	for i in $(find_files); do
		if [ "$F" != "" ] && [ "$F" != "$i" ]; then
			continue
		fi

		# search for more than one leading space, to ensure we use tabs
		if grep -q '^  ' "$i"; then
			echo "$i: MDX042: Leading spaces found instead of tabs" 1>&2
			echo "$i"
		fi

		# check the markdown format with the linter
		if ! $MDL --style "$STYLE" "$i" 1>&2; then
			echo "$i"
		fi
	done
)

# cleanup
if [ -e "$STYLE" ]; then
	rm "$STYLE"
fi

if [[ -n "${bad_files}" ]]; then
	# see a description of the rules at:
	# https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md
	fail_test "The following markdown files are not properly formatted:\n${bad_files}"
fi
echo 'PASS'

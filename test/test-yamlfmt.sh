#!/bin/bash
# check for any yaml files that aren't properly formatted

. test/util.sh

echo running test-yamlfmt.sh
set -o errexit
set -o nounset
set -o pipefail

if env | grep -q -e '^TRAVIS=true$' -e '^JENKINS_URL=' -e '^BUILD_TAG=jenkins'; then
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit 0
fi

ROOT=$(dirname "${BASH_SOURCE}")/..

RUBY=`which ruby 2>/dev/null`
if [ -z $RUBY ]; then
	fail_test "The 'ruby' utility can't be found."
fi

$RUBY -e "require 'yaml'" 2>/dev/null || fail_test "The ruby 'yaml' library can't be found."

if $RUBY -e "puts RUBY_VERSION" | grep -q ^1 ; then
	echo "SKIPPING - cannot test YAML formatting with Ruby 1.x"
	exit 0
fi

cd "${ROOT}"

find_files() {
	git ls-files | grep '\.yaml$'
}

bad_files=$(
	for i in $(find_files); do
		if ! diff -q <( ruby -e "require 'yaml'; puts YAML.load_file('$i').to_yaml.each_line.map(&:rstrip).join(10.chr)+10.chr" 2>/dev/null ) <( cat "$i" ) &>/dev/null; then
			echo "$i"
		fi
	done
)

if [[ -n "${bad_files}" ]]; then
	fail_test "The following yaml files are not properly formatted: ${bad_files}"
fi

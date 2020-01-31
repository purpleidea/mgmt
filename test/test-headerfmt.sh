#!/bin/bash
# check that headers are properly formatted

echo running "$0"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

FILE="main.go"	# file headers should match main.go
COUNT=0
while IFS='' read -r line; do	# find what header should look like
	echo "$line" | grep -q '^//' || break
	COUNT=`expr $COUNT + 1`
done < "$FILE"

find_files() {
	git ls-files | grep -E '\.go$|\.rl$' | grep -v '^examples/' | grep -v '^test/'
}

bad_files=$(
	for i in $(find_files); do
		if ! diff -q <( head -n $COUNT "$i" ) <( head -n $COUNT "$FILE" ) &>/dev/null; then
			echo "$i"
		fi
	done
)

if [[ -n "${bad_files}" ]]; then
	fail_test "The following file headers are not properly formatted: ${bad_files}"
fi
echo 'PASS'

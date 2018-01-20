#!/bin/bash

if [ "$1" = "" ] || [ "$1" = "--help" ]; then
	echo "usage: append standard header to file"
	echo "./$(basename "$0") <file> | --help"
	exit 1
fi

ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
FILE="${ROOT}/main.go"	# file headers should match main.go
COUNT=0
while IFS='' read -r line; do	# find what header should look like
	echo "$line" | grep -q '^//' || break
	COUNT=`expr $COUNT + 1`
done < "$FILE"
#cd "${ROOT}"

COUNT=`expr $COUNT + 1`	# add one extra newline

# detect if header is correct before blasting another one in
if diff -q <( head -n $COUNT "$1" ) <( head -n $COUNT "$FILE" ) &>/dev/null; then
	exit 0
fi

tmpfile=`mktemp`	# get a temp file
# the output of the main.go header, dumped onto the file
head -n $COUNT "$FILE" | cat - "$1" > "$tmpfile" && mv "$tmpfile" "$1"

#!/bin/bash

set -e -o pipefail

echo running "$(basename "$0")"

. test/util.sh

# test if we can build for all OSes and ARCHes.

tmpdir="`$mktemp --tmpdir -d tmp.XXX`"	# get a dir outside of the main package
if [[ ! "$tmpdir" =~ "/tmp" ]]; then
	echo "unexpected tmpdir in: ${tmpdir}"
	exit 99
fi
log="$tmpdir/$(basename $0 .sh).log"

set +e
make crossbuild &> "$log"

RET=$?
if [ ! $RET -eq 0 ]; then
	echo 'FAIL'
	cat "$log"
else
	echo 'PASS'
fi

if [ "$tmpdir" = "" ]; then
	echo "BUG, tried to delete empty string path"
	exit 99
fi
rm -rf "$tmpdir"
exit $RET

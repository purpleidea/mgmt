#!/usr/bin/env bash

set -e -o pipefail

echo running "$(basename "$0")"

. test/util.sh

# test if we can build for all OSes and ARCHes.

tmpdir="`$mktemp --tmpdir -d tmp.XXX`"	# get a dir outside of the main package
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
rm -rf "$tmpdir"
exit $RET

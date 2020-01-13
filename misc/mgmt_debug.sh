#!/bin/bash

# This is a helper script to run mgmt and capture and filter stack traces. Any
# time mgmt crashes with a trace, it will first be filtered, and then displayed
# with `less`.

f1=`mktemp /tmp/tmp.X'X'X`
f2=`mktemp /tmp/tmp.X'X'X`

# run the program until it ends
# XXX: we need an --ignore-exit signal blocker too so we can ^\
#$@ 2>&1 | tee --ignore-interrupts --ignore-exit "$f1"
$@ 2>&1 | sigtee "$f1"

# clean up when we're done
function cleanup {
	filter-golang-stack.py "$f1" > "$f2"
	if [ $? -eq 0 ]; then
		less "$f2"
	fi
	if [ "$1" = "--preserve" ]; then
		echo "logged:"
		echo "$f1"
		echo "$f2"
	else
		rm "$f2"
		rm "$f1"
	fi

}
trap cleanup EXIT

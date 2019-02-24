#!/bin/bash
# simple test loop runner, eg: ./looper.sh make test-shell-exec-fail

while true
do
	echo "running: $@"
	$@	# run some command
	ret=$?
	if [ $ret -ne 0 ]; then
		echo "failed with code: $ret"
		exit $ret
	fi
	echo
done

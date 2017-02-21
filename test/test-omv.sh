#!/bin/bash -i
# simple test harness for testing mgmt via omv
echo running test-omv.sh
CWD=`pwd`
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from test directory

# vtest+ tests
RET=0
for i in omv/*.yaml; do
	echo "running: vtest+ $i"
	vtest+ "$i"
	if [ $? -ne 0 ]; then
		RET=1
		break	# remove this if we should run all tests even if one fails
	fi
done

# return to original dir
cd "$CWD" >/dev/null
if [ ! $RET -eq 0 ]; then
	echo 'FAIL'
	exit $RET
fi
echo 'PASS'

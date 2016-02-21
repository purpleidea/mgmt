#!/bin/bash -i
# simple test harness for testing mgmt via omv
echo running test-omv.sh
CWD=`pwd`
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from test directory

# vtest+ tests
for i in omv/*.yaml; do
	echo "running: vtest+ $i"
	vtest+ "$i"
done

# return to original dir
cd "$CWD" >/dev/null

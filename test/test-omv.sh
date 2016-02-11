#!/bin/bash -ie
# simple test harness for testing mgmt via omv
CWD=`pwd`
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from test directory

# vtest+ tests
vtest+ omv/helloworld.yaml

# return to original dir
cd "$CWD" >/dev/null

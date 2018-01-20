#!/bin/bash
# simple test harness for testing mgmt
# NOTE: this will rm -rf /tmp/mgmt/

echo running test-shell.sh
set -o errexit
set -o pipefail

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh
cd - >/dev/null

if [ "$1" == "--help" ] || [ "$1" == "-h" ]; then
	echo -e "usage: ./"`basename $0`" [[--help] | <test>]"
	echo -e "where: <test> is empty to run all tests, or <file>.sh from shell/ dir"
	exit 1
fi

LINE=$(printf '=%.0s' `seq -s ' ' $(tput cols)`)	# a terminal width string
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from main mgmt directory
make build
MGMT="$DIR/test/shell/mgmt"
cp -a "$DIR/mgmt" "$MGMT"	# put a copy there
failures=""
count=0

# loop through tests
for i in $DIR/test/shell/*.sh; do
	[ -x "$i" ] || continue	# file must be executable
	ii=`basename "$i"`	# short name
	# if ARGV has test names, only execute those!
	if [ "$1" != '' ]; then
		[ "$ii" != "$1" ] && continue
	fi
	cd $DIR/test/shell/ >/dev/null	# shush the cd operation
	mkdir -p '/tmp/mgmt/'	# directory for mgmt to put files in
	#echo "Running: $ii"
	export MGMT_TMPDIR='/tmp/mgmt/'	# we can add to env like this
	count=`expr $count + 1`
	set +o errexit	# don't kill script on test failure
	out=$($i 2>&1)	# run and capture stdout & stderr
	e=$?	# save exit code
	set -o errexit	# re-enable killing on script failure
	cd - >/dev/null
	rm -rf '/tmp/mgmt/'	# clean up after test
	if [ $e -ne 0 ]; then
		echo -e "FAIL\t$ii"	# fail
		# store failures...
		failures=$(
			# prepend previous failures if any
			[ -n "${failures}" ] && echo "$failures" && echo "$LINE"
			echo "Script: $ii"
			# if we see 124, it might be the exit value of timeout!
			[ $e -eq 124 ] && echo "Exited: $e (timeout?)" || echo "Exited: $e"
			if [ "$out" = "" ]; then
				echo "Output: (empty!)"
			else
				echo "Output:"
				echo "$out"
			fi
		)
	else
		echo -e "ok\t$ii"	# pass
	fi
done

# clean up
rm -f "$MGMT"
make clean

if [ "$count" = '0' ]; then
	fail_test 'No tests were run!'
fi

# display errors
if [[ -n "${failures}" ]]; then
	echo 'FAIL'
	echo 'The following tests failed:'
	echo "${failures}"
	exit 1
fi
echo 'PASS'

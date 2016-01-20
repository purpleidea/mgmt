#!/bin/bash
# simple test harness for testing mgmt
# NOTE: this will rm -rf /tmp/mgmt/

set -o errexit
set -o nounset
set -o pipefail

LINE=$(printf '=%.0s' `seq -s ' ' $(tput cols)`)	# a terminal width string
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from main mgmt directory
make build
MGMT="$DIR/test/shell/mgmt"
cp -a "$DIR/mgmt" "$MGMT"	# put a copy there
failures=""

# loop through tests
for i in $DIR/test/shell/*.sh; do
	[ -x "$i" ] || continue	# file must be executable
	# TODO: if ARGV has test names, only execute those!
	ii=`basename "$i"`	# short name
	cd $DIR/test/shell/ >/dev/null	# shush the cd operation
	mkdir -p '/tmp/mgmt/'	# directory for mgmt to put files in
	#echo "Running: $ii"
	set +o errexit	# don't kill script on test failure
	export MGMT_TMPDIR='/tmp/mgmt/'	# we can add to env like this
	out=$($i 2>&1)	# run and capture stdout & stderr
	e=$?	# save exit code
	set -o errexit	# re-enable killing on script failure
	cd - >/dev/null
	rm -rf '/tmp/mgmt/'	# clean up after test
	if [ $e -ne 0 ]; then
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

# display errors
if [[ -n "${failures}" ]]; then
	echo 'FAIL'
	echo 'The following tests failed:'
	echo "${failures}"
	exit 1
fi
echo PASS

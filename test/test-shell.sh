#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# simple test harness for testing mgmt
# NOTE: this will rm -rf /tmp/mgmt/
################################################################################

if [ "${1:-""}" == "--help" ] || [ "${1:-""}" == "-h" ]; then
	echo -e "usage: ./"`basename $0`" [[--help] | <test>]"
	echo -e "where: <test> is empty to run all tests, or <file>.sh from shell/ dir"
	exit 1
fi

# Ensure ./mgmt exists.
make build

# Start clean.
rm -fr /tmp/mgmt*

# loop through tests
for i in ./test/shell/*.sh; do
	[ -x "$i" ] || continue	# file must be executable

	# if ARGV has test names, only execute those!
	if [[ -n "${1:-""}" ]]; then
		continue
	fi

	mkdir -p /tmp/mgmt/

	if ! MGMT_TMPDIR='/tmp/mgmt/' $i; then
		warn "If exit code is 124, it could mean timeout."
		if [[ -n "${failures}" ]]; then
			failures="$(echo -e "${failures}\\n$i")"
		else
			failures="$i"
		fi
	fi

	rm -rf '/tmp/mgmt/'
done

# clean up
make clean

# display errors
if [[ -n "${failures}" ]]; then
	err 'The following tests failed:'
	indent "${failures}"
	exit 1
fi

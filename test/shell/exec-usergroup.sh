#!/bin/bash

. "$(dirname "$0")/../util.sh"

set -x
set -o pipefail

if ! timeout 1s sudo -A true; then
	echo "sudo disabled: not checking exec user and group"
	exit
fi

BASE_PATH="/tmp/mgmt/"
BASE_PATH_TEST="${BASE_PATH}test-exec-usergroup/"
# on Fedora, it's nobody while on ubuntu it's nogroup
GROUP="nogroup"
if grep -q nobody /etc/group; then
	GROUP="nobody"
fi

function setup {
	mkdir -p "${BASE_PATH_TEST}"
	sudo -A chown nobody:${GROUP} "${BASE_PATH_TEST}"
	sudo -A chmod ug=rwx,o=rx "${BASE_PATH_TEST}"
}

function cleanup {
	sudo -A rm -rf "${BASE_PATH_TEST}"
}

# run_test will run each test. It takes 3 parameters:
# - $1: graph (e.g. exec-usergroup-nobody.yaml)
# - $2: user to be tested (e.g. nobody or "")
# - $3: group to be tested (e.g. nobody or "")
function run_usergroup_test() {
	graph=$1
	user=$2
	group=$3

	setup

	# run till completion
	sudo -A $TIMEOUT "$MGMT" run --converged-timeout=15 --no-watch --tmp-prefix yaml ./exec-usergroup/${graph} &
	pid=$!
	wait $pid	# get exit status
	e=$?

	# tests
	test -e "${BASE_PATH_TEST}/result-exec-usergroup"
	if [ $? != 0 ]; then
		echo "${BASE_PATH_TEST}result-exec-usergroup has not been created"
		exit 1
	fi
	if [ "${user}" != "" ]; then
		test $(stat -c%U "${BASE_PATH_TEST}/result-exec-usergroup") = $user
		if [ $? != 0 ]; then
			echo "${BASE_PATH_TEST}result-exec-usergroup owner is not ${user}"
			exit 1
		fi
	fi
	if [ "${group}" != "" ]; then
		test $(stat -c%G "${BASE_PATH_TEST}/result-exec-usergroup") = $group
		if [ $? != 0 ]; then
			echo "${BASE_PATH_TEST}result-exec-usergroup group is not ${group}"
			exit 1
		fi
	fi

	cleanup
}

# ensure the workspace is clean
cleanup

# run_usergroup_test <yaml file in ./exec-usergroup> <user to test> <group to test>
run_usergroup_test "exec-usergroup-${GROUP}.yaml" "nobody" "${GROUP}"
run_usergroup_test "exec-usergroup-user.yaml" "nobody" ""
run_usergroup_test "exec-usergroup-group-${GROUP}.yaml" "" "${GROUP}"

# avoid race against rm command from the shell test wrapper
sleep 1

#!/bin/bash

# common settings and functions for test scripts

# get the fully expanded path of the project directory
ROOT="$(realpath "$(dirname "$(realpath "${BASH_SOURCE}")")/..")"

# absolute location to freshly build binary to be used for testing
export MGMT="$ROOT/mgmt"

if [[ $(uname) == "Darwin" ]] ; then
	export timeout="gtimeout"
	export mktemp="gmktemp"
else
	export timeout="timeout"
	export mktemp="mktemp"
fi

fail_test()
{
	redb "FAIL: " "$@"
	exit 1
}

function run-test()
{
	"$@" || failures=$( [ -n "$failures" ] && echo "$failures\\n$@" || echo "$@" )
}

# enable colors if we run in a colorful terminal (ie: xterm-256color, xterm-color)
# overwritable by setting environment variable ENABLE_COLORS empty `export ENABLE_COLORS=`
if [[ "$TERM" =~ .*color.* ]];then
	colors="${ENABLE_COLORS-1}"
else
	colors=""
fi

# colours https://gist.github.com/daytonn/8677243
end="\033[0m"
red="\033[0;31m"
redb="\033[1;31m"
green="\033[0;32m"
greenb="\033[1;32m"
yellow="\033[0;33m"
blue="\033[0;34m"

function red { test -z "$colors" && echo "${1}" || echo -e "${red}${1}${end}"; }
function redb { test -z "$colors" && echo "${1}" || echo -e "${redb}${1}${end}"; }
function green { test -z "$colors" && echo "${1}" || echo -e "${green}${1}${end}"; }
function greenb { test -z "$colors" && echo "${1}" || echo -e "${greenb}${1}${end}"; }
function yellow { test -z "$colors" && echo "${1}" || echo -e "${yellow}${1}${end}"; }
function blue { test -z "$colors" && echo "${1}" || echo -e "${blue}${1}${end}"; }

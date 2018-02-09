# common settings and functions for test scripts

if [[ $(uname) == "Darwin" ]] ; then
	export timeout="gtimeout"
	export mktemp="gmktemp"
else
	export timeout="timeout"
	export mktemp="mktemp"
fi

fail_test()
{
	echo "FAIL: $@"
	exit 1
}

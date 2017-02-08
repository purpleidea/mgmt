# common settings and functions for test scripts

fail_test()
{
	echo "FAIL: $@"
	exit 1
}

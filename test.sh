#!/bin/bash -e

# test suite...
echo running test.sh

# ensure there is no trailing whitespace or other whitespace errors
git diff-tree --check $(git hash-object -t tree /dev/null) HEAD

# ensure entries to authors file are sorted
start=$(($(grep -n '^[[:space:]]*$' AUTHORS | awk -F ':' '{print $1}' | head -1) + 1))
diff <(tail -n +$start AUTHORS | sort) <(tail -n +$start AUTHORS)

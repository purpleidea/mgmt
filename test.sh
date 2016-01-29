#!/bin/bash -e

# test suite...
echo running test.sh

# ensure there is no trailing whitespace or other whitespace errors
git diff-tree --check $(git hash-object -t tree /dev/null) HEAD

# ensure entries to authors file are sorted
start=$(($(grep -n '^[[:space:]]*$' AUTHORS | awk -F ':' '{print $1}' | head -1) + 1))
diff <(tail -n +$start AUTHORS | sort) <(tail -n +$start AUTHORS)

./test/test-gofmt.sh
./test/test-yamlfmt.sh
go test
echo running go vet	# since it doesn't output an ok message on pass
go vet && echo PASS

# do these longer tests only when running on ci
if env | grep -q -e '^TRAVIS=true$' -e '^JENKINS_URL='; then
	go test -race
	./test/test-shell.sh
fi

# run omv tests on jenkins physical hosts only
if env | grep -q -e '^JENKINS_URL='; then
	./test/test-omv.sh
fi

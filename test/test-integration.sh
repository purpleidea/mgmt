#!/bin/bash

# this file exists to that bash completion for test names works

echo running "$0" "$@"

#ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

# this test is handled as a special `go test` test
exec test/test-gotest.sh --integration $@

#!/bin/bash
# check that go vet passes
echo running test-govet.sh
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}"

go vet && echo PASS || exit 1	# since it doesn't output an ok message on pass
grep 'log.' *.go | grep '\\n"' && exit 1 || echo PASS	# no \n needed in log.Printf()

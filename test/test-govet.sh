#!/bin/bash
# check that go vet passes

. test/util.sh

echo running test-govet.sh
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}"

for file in `find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*'`; do
	go vet "$file" && echo PASS || fail_test "go vet did not pass"	# since it doesn't output an ok message on pass
	grep 'log.' "$file" | grep '\\n"' && fail_test 'no \n needed in log.Printf()' || echo PASS	# no \n needed in log.Printf()
	grep 'case _ = <-' "$file" && fail_test 'case _ = <- can be simplified to: case <-' || echo PASS	# this can be simplified
	grep -Ei "[\/]+[\/]+[ ]*+(FIXME[^:]|TODO[^:]|XXX[^:])" "$file" && fail_test 'Token is missing a colon' || echo PASS	# tokens must end with a colon
done

#!/bin/bash
# check that go vet passes
echo running test-govet.sh
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}"

for file in `find . -maxdepth 3 -type f -name '*.go' -not -path './old/*' -not -path './tmp/*'`; do
	go vet "$file" && echo PASS || exit 1	# since it doesn't output an ok message on pass
	grep 'log.' "$file" | grep '\\n"' && echo 'no \n needed in log.Printf()' && exit 1 || echo PASS	# no \n needed in log.Printf()
	grep 'case _ = <-' "$file" && echo 'case _ = <- can be simplified to: case <-' && exit 1 || echo PASS	# this can be simplified
done

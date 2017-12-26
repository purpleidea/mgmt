#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

go build -i -o libmgmt test/shell/libmgmt-change2.go
# this example should change graphs frequently, and then shutdown...
$timeout --kill-after=30s 20s ./libmgmt &
pid=$!
wait $pid	# get exit status
e=$?
rm libmgmt	# cleanup build artefact
exit $e

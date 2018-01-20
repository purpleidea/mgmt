#!/bin/bash -e

# XXX: this has not been updated to latest GAPI/Deploy API. Patches welcome!
exit 0

go build -i -o libmgmt libmgmt-change1.go
# this example should change graphs frequently, and then shutdown...
$timeout --kill-after=30s 20s ./libmgmt &
pid=$!
wait $pid	# get exit status
e=$?
rm libmgmt	# cleanup build artefact
exit $e

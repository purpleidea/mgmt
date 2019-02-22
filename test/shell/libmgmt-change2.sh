#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

# XXX: this has not been updated to latest GAPI/Deploy API. Patches welcome!
exit 0

go build -i -o libmgmt libmgmt-change2.go
# this example should change graphs frequently, and then shutdown...
$TIMEOUT ./libmgmt &
pid=$!
wait $pid	# get exit status
e=$?
rm libmgmt	# cleanup build artefact
exit $e

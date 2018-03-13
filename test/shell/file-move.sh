#!/bin/bash -e

mkdir -p /tmp/mgmt/
rm /tmp/mgmt/f1 || true

# run empty graph, with prometheus support
$timeout --kill-after=60s 55s "$MGMT" run --tmp-prefix --yaml=file-move.yaml 2>&1 | tee /tmp/mgmt/file-move.log &
pid=$!
sleep 5s	# let it converge

initial=$(grep -c 'file\[file1\]: resource: contentCheckApply(true)' /tmp/mgmt/file-move.log)

mv /tmp/mgmt/f1 /tmp/mgmt/f2

sleep 3s

after_move_count=$(grep -c 'file\[file1\]: resource: contentCheckApply(true)' /tmp/mgmt/file-move.log)

sleep 3s

echo f2 > /tmp/mgmt/f2

after_moved_file_count=$(grep -c 'file\[file1\]: resource: contentCheckApply(true)' /tmp/mgmt/file-move.log)

if [[ ${after_move_count} -le ${initial} ]]
then
	echo 'File move did not trigger a CheckApply'
	exit 1
fi

if [[ ${after_moved_file_count} -gt ${after_move_count} ]]
then
	echo 'Changing the moved file did trigger a CheckApply'
	exit 1
fi

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

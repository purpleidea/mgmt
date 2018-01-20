#!/bin/bash -e

exit 0	# TODO: this test needs to be update to use deploys instead

#if env | grep -q -e '^TRAVIS=true$'; then
#	# inotify doesn't seem to work properly on travis
#	echo "Travis and Jenkins give wonky results here, skipping test!"
#	exit
#fi

if [ -z $timeout ]; then
	timeout='timeout'
fi

# set the config file
cp -a yaml-change1a.yaml /tmp/mgmt/yaml-change.yaml
$timeout --kill-after=30s 20s ./mgmt run --yaml /tmp/mgmt/yaml-change.yaml --tmp-prefix &
pid=$!
sleep 5s	# let it converge
grep -q 'hello world' /tmp/mgmt/change1	# check contents are correct

cp -a yaml-change1b.yaml /tmp/mgmt/yaml-change.yaml	# change the config file
sleep 2s	# let it converge
grep -q 'goodbye world' /tmp/mgmt/change1	# check new contents are correct

cp -a yaml-change1a.yaml /tmp/mgmt/yaml-change.yaml	# change the config file
sleep 2s	# let it converge
grep -q 'hello world' /tmp/mgmt/change1	# check contents are correct again

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

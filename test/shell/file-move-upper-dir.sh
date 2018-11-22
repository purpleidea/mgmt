#!/bin/bash -e

# FIXME: test for #124 --- Disabled for now

exit 0

mkdir -p /tmp/mgmt/a/b/c/

# run empty graph, with prometheus support
$timeout --kill-after=60s 55s "$MGMT" run --tmp-prefix yaml --yaml=examples/deep-dirs.yaml &
pid=$!
sleep 10s	# let it converge

grep f1 /tmp/mgmt/a/b/c/f1

echo 'f2!' > /tmp/mgmt/a/b/c/f1

grep f1 /tmp/mgmt/a/b/c/f1

rm -rf /tmp/mgmt/a/b/C/ || true
mv /tmp/mgmt/a/b/c /tmp/mgmt/a/b/C/

mkdir -p /tmp/mgmt/a/b/c

echo 'f2!' > /tmp/mgmt/a/b/c/f1

grep f1 /tmp/mgmt/a/b/c/f1

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

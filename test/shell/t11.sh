#!/bin/bash -e

# create a file without write access, so mgmt can
# not overwrite it
mkdir -p /tmp/mgmt
rm -f /tmp/mgmt/hello || true
> /tmp/mgmt/hello
chmod a-w /tmp/mgmt/hello

# run file graph, with prometheus support
timeout --kill-after=50s 30s ./mgmt run --tmp-prefix --prometheus --yaml examples/file4.yaml &
pid=$!
sleep 5s	# let it converge

# Check that the file metric is exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_resources{type="File"} 1$'

# Check that the soft failure is exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_failures{kind="soft",type="File"} 1$'

# Check that the total of soft failures is exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_failures_total{kind="soft",type="File"} [0-9]\+$'

# Check that the total of soft failures is not 1
if curl 127.0.0.1:9233/metrics | grep '^mgmt_failures_total{kind="soft",type="File"} 1$'
then
    false
fi

# wait for the retries to be done
sleep 15

# Check that the file metric is exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_resources{type="File"} 1$'

# Check that the soft failure is not exposed
if curl 127.0.0.1:9233/metrics | grep '^mgmt_failures{kind="soft",type="File"}'
then
    false
fi

# Check that the hard failur is not exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_failures{kind="hard",type="File"} 1$'

# Check that the total of hard failures is exposed
curl 127.0.0.1:9233/metrics | grep '^mgmt_failures_total{kind="hard",type="File"} 1$'

killall -SIGINT mgmt	# send ^C to exit mgmt
wait $pid	# get exit status
exit $?

# NOTE: boiler plate to run etcd; source with: . etcd.sh; should NOT be +x
cleanup ()
{
	killall etcd || killall -9 etcd || true	# kill etcd
	rm -rf /tmp/etcd/
}
trap cleanup INT QUIT TERM EXIT ERR
mkdir -p /tmp/etcd/
cd /tmp/etcd/ >/dev/null	# shush the cd operation
etcd &	# start etcd as job # 1
sleep 1s	# let etcd startup
cd - >/dev/null

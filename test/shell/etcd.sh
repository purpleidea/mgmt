# NOTE: boiler plate to run etcd; source with: . etcd.sh; should NOT be +x
cleanup ()
{
	echo "cleanup: $1"
	killall etcd || killall -9 etcd || true	# kill etcd
	rm -rf /tmp/etcd/
}

trap_with_arg() {
	func="$1"
	shift
	for sig in "$@"
	do
		trap "$func $sig" "$sig"
	done
}

trap_with_arg cleanup INT QUIT TERM EXIT	#	ERR
mkdir -p /tmp/etcd/
cd /tmp/etcd/ >/dev/null	# shush the cd operation
etcd &	# start etcd as job # 1
sleep 1s	# let etcd startup
cd - >/dev/null

#!/bin/bash -e

. "$(dirname "$0")/../util.sh"

if env | grep -q -e '^TRAVIS=true$'; then
	# this often fails in travis with: `address already in use`
	echo "Travis gives wonky results here, skipping test!"
	exit
fi

set -x
set -o pipefail

# can't test net without sudo
if ! timeout 1s sudo -A true; then
	echo "sudo disabled: not checking net"
	exit
fi

# values from net0.yaml
IFACE="mgmtnet0"
ADDR="192.168.42.13/24"

# bad value used to test events
BADADDR="10.0.0.254/24"

# is_up returns 0 if the interface ($1) is up, and 1 if it is down.
function is_up {
	if [ -z  "$(ip link show $1 up)" ]; then
		return 1
	fi
}

# has_addr returns 0 if the iface ($1) has the addr ($2), and 1 if it doesn't.
function has_addr {
	if [ -z "$(ip addr show $1 | grep $2)" ]; then
		return 1
	fi
}

# clean up when we're done
function cleanup {
	sudo rm  /etc/systemd/network/mgmt-${IFACE}.network || true
	sudo ip link del $IFACE || true
}
trap cleanup EXIT

# make sure there's somewhere to save the unit file
sudo mkdir -p /etc/systemd/network

# add a dummy link for tests
sudo ip link add $IFACE type dummy || true

# run mgmt net res with $IFACE and $ADDR set as above
sudo -A $TIMEOUT "$MGMT" run --converged-timeout=5 --tmp-prefix lang ./net0.mcl &
pid1=$!

# give the engine time to start up
sleep 5

# make sure the interface is up
if ! is_up $IFACE; then
	echo "failed to bring up $IFACE"
	exit 1
fi
# check the address
if ! has_addr $IFACE $ADDR; then
	echo "failed to set addr: $ADDR on $IFACE"
	exit 1
fi

# make sure the interface comes up if we set it down
sudo ip link set down $IFACE
if ! is_up $IFACE; then
	echo "failed to bring $IFACE back up"
	exit 1
fi
# make sure the address is replaced if we delete it
sudo ip addr del $ADDR dev $IFACE
if ! has_addr $IFACE $ADDR; then
	echo "failed to replace addr: $ADDR on $IFACE"
	exit 1
fi

# add a bad address and make sure it is removed
sudo ip addr add $BADADDR dev $IFACE
if has_addr $IFACE $BADADDR; then
	echo "failed to remove addr: $BADADDR from $IFACE"
	exit 1
fi

wait $pid1
e1=$?

# run mgmt net res with $IFACE state => "down"
sudo -A $TIMEOUT "$MGMT" run --converged-timeout=5 --tmp-prefix lang ./net1.mcl &

# give the engine time to start up
sleep 5

# make sure the interface is down
if is_up $IFACE; then
	echo "failed to bring down $IFACE"
	exit 1
fi

# bring up the interface and make sure it's brought back down
sudo ip link set up $IFACE
if is_up $IFACE; then
	echo "failed to bring $IFACE back down"
	exit 1
fi

wait $pid2
e2=$?

exit $(($e1+$e2))

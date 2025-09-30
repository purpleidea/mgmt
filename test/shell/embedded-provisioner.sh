#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# misc bash functions, eg: repeat "#" 42
function repeat() {
	for ((i=0; i<$2; ++i)); do echo -n "$1"; done
	echo
}

set -x

# run unification with a dummy password
eth=$(for i in /sys/class/net/*; do d=${i##*/}; [[ "$d" != "lo" && $(<"$i/type") -eq 1 ]] && echo "$d" && break; done) # get the first ethernet device

$TIMEOUT "$MGMT" provisioner --interface $eth --only-unify --password $(repeat "#" 106) &
pid=$!
wait $pid	# get exit status
e=$?

exit $e

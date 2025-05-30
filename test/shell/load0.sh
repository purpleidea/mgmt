#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

# TODO: do loadavg calls work properly on macOS in travis?

set -o errexit
set -o pipefail

# Expected load average values eg: load average: 1.64306640625, 1.8076171875, 1.82958984375
# High precision results are preferred (more than the 2 digits in /proc/loadavg at least).
# Precision varies (eg: 4, 9 or 11 digits). Hence no strict check for precision but
# anything above 3 will do. It is assumed we will hardly ever get a precision lower than 3 digits
# from the current implementations. Otherwise this test would need to be revised.
# We once got: 'load average: 0, 0.28515625, 0.349609375'.
regex="load average: ([0-9]\.[0-9]{3,})|0, [0-9]\.[0-9]{3,}, [0-9]\.[0-9]{3,}"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"
if [[ ! "$tmpdir" =~ "/tmp" ]]; then
	echo "unexpected tmpdir in: ${tmpdir}"
	exit 99
fi

cat > "$tmpdir/load0.mcl" <<EOF
import "fmt"
import "sys"

\$theload = sys.load()

\$x1 = \$theload->x1
\$x5 = \$theload->x5
\$x15 = \$theload->x15

file "${tmpdir}/loadavg" {
	content => fmt.printf("load average: %f, %f, %f\n", \$x1, \$x5, \$x15),
	state => \$const.res.file.state.exists,
}
EOF

$TIMEOUT "$MGMT" run --tmp-prefix --converged-timeout=15 lang "$tmpdir/load0.mcl"  &
pid=$!
wait $pid	# get exit status
e=$?

set +e
egrep "$regex" "$tmpdir/loadavg" || fail_test "Could not match $tmpdir/loadavg to '$regex'. Got: '`cat $tmpdir/loadavg`'."

if [ "$tmpdir" = "" ]; then
	echo "BUG, tried to delete empty string path"
	exit 99
fi
# cleanup if everything went well
rm -r "$tmpdir"

exit $e

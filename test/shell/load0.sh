#!/bin/bash -e

if env | grep -q -e '^TRAVIS=true$' && [[ $(uname) == "Darwin" ]]; then
	# loadavg glibc calls don't seem to work properly on osx OS in travis
	echo "Travis and Jenkins give wonky results here, skipping test!"
	exit
fi

set -o errexit
set -o pipefail

# Expected load average values eg: load average: 1.64306640625, 1.8076171875, 1.82958984375
# High precision results are preferred (more than the 2 digits in /proc/loadavg at least).
# Precision varies (eg: 4, 9 or 11 digits). Hence no strict check for precision but
# anything above 3 will do. It is assumed we will hardly ever get a precision lower than 3 digits
# from the current implementations. Otherwise this test would need to be revised.
regex="load average: [0-9]\,[0-9]{3,}, [0-9]\,[0-9]{3,}, [0-9]\,[0-9]{3,}"

tmpdir="$($mktemp --tmpdir -d tmp.XXX)"

# macOS workaround https://github.com/purpleidea/mgmt/issues/33
touch "$tmpdir/loadavg"

cat > "$tmpdir/load0.mcl" <<EOF
\$theload = load()

\$x1 = structlookup(\$theload, "x1")
\$x5 = structlookup(\$theload, "x5")
\$x15 = structlookup(\$theload, "x15")

file "${tmpdir}/loadavg" {
	content => printf("load average: %f, %f, %f", \$x1, \$x5, \$x15),
	state => "exists",
}
EOF

$timeout --kill-after=60s 55s "$MGMT" run --tmp-prefix --converged-timeout=1 --lang "$tmpdir/load0.mcl"  &
pid=$!
wait $pid	# get exit status
e=$?

set +e
egrep "$regex" "$tmpdir/loadavg" || fail_test "Could not match $tmpdir/loadavg to '$regex'."

# cleanup if everything went well
rm -r "$tmpdir"

exit $e

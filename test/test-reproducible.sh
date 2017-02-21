#!/bin/bash
# simple test for reproducibility, probably needs major improvements
echo running test-reproducible.sh
set -o errexit
set -o pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "$DIR" >/dev/null	# work from main mgmt directory
make build
T=`mktemp --tmpdir -d tmp.XXX`
cp -a ./mgmt "$T"/mgmt.1
make clean
make build
cp -a ./mgmt "$T"/mgmt.2

# size comparison test
[ `stat -c '%s' "$T"/mgmt.1` -eq `stat -c '%s' "$T"/mgmt.2` ] || failures="Size of binary was not reproducible"

# sha1sum test
sha1sum "$T"/mgmt.1 > "$T"/mgmt.SHA1SUMS.1
sha1sum "$T"/mgmt.2 > "$T"/mgmt.SHA1SUMS.2
cat "$T"/mgmt.SHA1SUMS.1 | sed 's/mgmt\.1/mgmt\.X/' > "$T"/mgmt.SHA1SUMS.1X
cat "$T"/mgmt.SHA1SUMS.2 | sed 's/mgmt\.2/mgmt\.X/' > "$T"/mgmt.SHA1SUMS.2X
diff -q "$T"/mgmt.SHA1SUMS.1X "$T"/mgmt.SHA1SUMS.2X || failures=$( [ -n "${failures}" ] && echo "$failures" ; echo "SHA1SUM of binary was not reproducible" )

# clean up
if [ "$T" != '' ]; then
	rm -rf "$T"
fi
make clean

# display errors
if [[ -n "${failures}" ]]; then
	echo 'FAIL'
	echo 'The following tests failed:'
	echo "${failures}"
	exit 1
fi
echo 'PASS'

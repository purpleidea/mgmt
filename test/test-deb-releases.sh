#!/bin/bash

# test if .deb package installs and mgmt runs on supported LTS Debian/Ubuntu

set -o errexit
set -o pipefail

releases=(ubuntu:18.04 debian:9 ubuntu:16.04 debian:8 )

. "$(dirname "$0")/util.sh"

version=$(git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --abbrev=0)
release_deb="releases/$version/deb/mgmt_${version}_amd64.deb"
testcmd="set -x; apt-get update >/dev/null && \
	dpkg -i /$release_deb 2>/dev/null; \
	apt-get install -yqqf >/dev/null && \
	mgmt --version"

# test if release build is available
if ! test -f "$release_deb"; then
	fail_test "Release file to test ($release_deb) is not available!"
fi

failures=""

for release in "${releases[@]}";do
	echo "Testing: $release"
	if ! docker run -ti --rm \
			-v "$PWD/releases/:/releases/" \
			"$release" /bin/sh -c "$testcmd";
	then
		failures="$failures $release"
	fi
done

if [[ -n "${failures}" ]]; then
	echo 'FAIL'
	echo 'The following tests failed:'
	echo "${failures}"
	exit 1
fi
echo 'PASS'

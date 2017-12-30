#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# FIXME: execute "go test" outside of /tmp
# so the test harness works in environments where /tmp is mounted noexec.
if [[ -n "$(awk '$2 ~ /^\/tmp$/ && $4 ~ /noexec/ {print $0}' /proc/mounts)" ]]; then
	err "/tmp is mounted noexec; $0 is guaranteed to fail."
	exit 1
fi

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

declare -i RC=0

base=$(go list .)
for pkg in `go list ./... | grep -v "^${base}/vendor/" | grep -v "^${base}/examples/" | grep -v "^${base}/test/" | grep -v "^${base}/old/" | grep -v "^${base}/tmp/"`; do
	info "Testing: $pkg"

	# FIXME: can we capture and output the stderr from these tests too?
	run-test go test "$pkg"
	RC=$?

	# Run and report test with -race option.
	if [[ "${1:-""}" = "--race" ]]; then
		run-test go test -race "$pkg"
		RC=$?
	fi
done

if [[ -n "$failures" ]]; then
	err 'The following `go test` runs have failed:'
	indent "$failures"
fi
exit ${RC}

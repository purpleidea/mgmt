#!/usr/bin/env bash
# check that our examples still build, even if we don't run them here

# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"

failures=''

# Test examples/lang/ directory to see if the .mcl files compile correctly.

find_mcl_examples() {
	repo_files | grep '\.mcl$' | grep '^examples/lang/' | grep -v 'modules/'
}

mcl_jobs=${MGMT_TEST_JOBS:-}
if [[ -z $mcl_jobs ]]; then
	mcl_jobs=$(getconf _NPROCESSORS_ONLN 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || true)
fi
case "$mcl_jobs" in
'' | 0 | *[!0-9]*) mcl_jobs=1 ;;
esac
if [[ -z ${MGMT_TEST_JOBS:-} ]]; then
	# These short checks benefit from oversubscription; 8x was near the
	# measured throughput plateau.
	mcl_jobs=$((mcl_jobs * 8))
fi

# Each file is an independent mcl program. Check several in parallel, while
# keeping the number of memory-heavy mgmt processes bounded.
failures=$(
	# bash's $0 argument is underscore so xargs appends each file as $1.
	find_mcl_examples | xargs -n 1 -P "$mcl_jobs" bash -c '
		file=$1
		"$MGMT" check --tmp-prefix lang --skip-fmt "$file" &>/dev/null || echo "./mgmt check --tmp-prefix lang --skip-fmt $file"
	' _ # arg0 is not needed
)

buildout='test-examples.out'
# make symlink to outside of package
linkto="`pwd`/examples/lib/"
tmpdir="`$mktemp --tmpdir -d tmp.XXX`"	# get a dir outside of the main package
cd "$tmpdir"
ln -s "$linkto"	# symlink outside of dir
cd `basename "$linkto"`

# loop through individual *.go files in working dir
for file in `find . -maxdepth 9 \( -type f -o -type l \) -name '*.go'`; do
	# skip broken reference examples; they have this tag we can search for
	if grep -q '^//go:build ignore$' "$file"; then
		continue
	fi
	#echo "running test on: $file"
	run-test go build -o "$buildout" "$file" || fail_test "could not build: $file"
done
rm -f "$buildout"	# clean up build mess

cd - >/dev/null	# back to tmp dir
rm `basename "$linkto"`
cd ..
rmdir "$tmpdir"	# cleanup

if [[ -n "$failures" ]]; then
	echo 'FAIL'
	echo "The following tests (in: ${linkto}) have failed:"
	echo -e "$failures"
	echo
	exit 1
fi
echo 'PASS'

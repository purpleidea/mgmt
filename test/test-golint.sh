#!/bin/bash
# check that go lint passes or doesn't get worse by some threshold

. test/util.sh

echo running test-golint.sh
# TODO: output a diff of what has changed in the golint output
# FIXME: test a range of commits, since only the last patch is checked here
PREVIOUS='HEAD^'
CURRENT='HEAD'
THRESHOLD=15	# percent problems per new LOC
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

# if this branch has more than one commit as compared to master, diff to that
# note: this is a cheap way to avoid doing a fancy succession of golint's...
HACK=''
COMMITS="`git rev-list --count $CURRENT ^master`"	# commit delta to master
# avoid: bad revision '^master' on travis for unknown reason :(
if [ "$COMMITS" != "" ] && [ "$COMMITS" -gt "1" ]; then
	PREVIOUS='master'
	HACK="yes"
fi

LINT=`find . -maxdepth 3 -iname '*.go' -not -path './old/*' -not -path './tmp/*' -exec golint {} \;`	# current golint output
COUNT=`echo -e "$LINT" | wc -l`	# number of golint problems in current branch
[ "$LINT" = "" ] && echo PASS && exit	# everything is "perfect"
echo "$LINT"	# display the issues

T=`mktemp --tmpdir -d tmp.XXX`
[ "$T" = "" ] && fail_test "Could not create tmpdir"
cd $T || fail_test "Could not change into tmpdir $T"
git clone --recursive "${ROOT}" 2>/dev/null	# make a copy
cd "`basename ${ROOT}`" >/dev/null || fail_test "Could not determine basename for the repo root '$ROOT'"
if [ "$HACK" != "" ]; then
	# ensure master branch really exists when cloning from a branched repo!
	git checkout master &>/dev/null && git checkout - &>/dev/null
fi

DIFF1=0
NUMSTAT1=`git diff "$PREVIOUS" "$CURRENT" --numstat`	# numstat diff since previous commit
while read -r line; do
	add=`echo "$line" | cut -f1`
	# TODO: should we only count added lines, or count the difference?
	sum="$add"
	#del=`echo "$line" | cut -f2`
	#sum=`expr $add - $del`
	DIFF1=`expr $DIFF1 + $sum`
done <<< "$NUMSTAT1"	# three < is the secret to putting a variable into read

git checkout "$PREVIOUS" &>/dev/null	# previous commit
LINT1=`find . -maxdepth 3 -iname '*.go' -not -path './old/*' -not -path './tmp/*' -exec golint {} \;`
COUNT1=`echo -e "$LINT1" | wc -l`	# number of golint problems in older branch

# clean up
cd "$XPWD" >/dev/null
rm -rf "$T"

DELTA=$(printf "%.0f\n" `echo - | awk "{ print (($COUNT - $COUNT1) / $DIFF1) * 100 }"`)

echo "Lines of code: $DIFF1"
echo "Prev. # of issues: $COUNT1"
echo "Curr. # of issues: $COUNT"
echo "Issue count delta is: $DELTA %"
if [ "$DELTA" -gt "$THRESHOLD" ]; then
	echo "Maximum threshold is: $THRESHOLD %"
	fail_test "`golint` - FAILED"
fi
echo PASS

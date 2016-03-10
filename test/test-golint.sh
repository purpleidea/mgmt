#!/bin/bash
# check that go lint passes or doesn't get worse by some threshold
echo running test-golint.sh
# FIXME: test a range of commits, since only the last patch is checked here
PREVIOUS='HEAD^'
CURRENT='HEAD'
THRESHOLD=15	# percent problems per new LOC
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

LINT=`golint`	# current golint output
COUNT=`echo -e "$LINT" | wc -l`	# number of golint problems in current branch

T=`mktemp --tmpdir -d tmp.XXX`
[ "$T" = "" ] && exit 1
cd $T || exit 1
git clone --recursive "${ROOT}" 2>/dev/null	# make a copy
cd "`basename ${ROOT}`" >/dev/null || exit 1

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

git checkout "$PREVIOUS" 2>/dev/null	# previous commit
LINT1=`golint`
COUNT1=`echo -e "$LINT1" | wc -l`	# number of golint problems in older branch

# clean up
cd "$XPWD" >/dev/null
rm -rf "$T"

[ "$LINT1" = "" ] && echo PASS && exit	# everything is "perfect"
DELTA=$(printf "%.0f\n" `echo - | awk "{ print (($COUNT1 - $COUNT) / $DIFF1) * 100 }"`)

echo "Lines of code: $DIFF1"
echo "Prev. # of issues: $COUNT"
echo "Curr. # of issues: $COUNT1"
echo "Issue count delta is: $DELTA %"
if [ "$DELTA" -gt "$THRESHOLD" ]; then
	echo "Maximum threshold is: $THRESHOLD %"
	echo '`golint` FAIL'
	exit 1
fi
echo PASS

#!/usr/bin/env -S bash -e

. "$(dirname "$0")/../util.sh"

if in_env github; then
	# TODO: consider debugging this
	echo "This is failing in github, skipping test!"
	exit
fi

# should take a few seconds plus converged timeout, and test we don't hang!
# TODO: should we return a different exit code if the resources fail?
# TODO: should we be converged if one of the resources has permanently failed?
$TIMEOUT "$MGMT" run --converged-timeout=15 --no-watch --no-pgp --tmp-prefix yaml exec-fail.yaml &
# there's no ^C sent... it should shutdown on its own
pid=$!
wait $pid	# get exit status
exit $?

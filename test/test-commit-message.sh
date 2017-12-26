#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# Test each commit message.
################################################################################

readonly travis_regex='^\([a-z0-9]\(\(, \)\|[a-z0-9]\)\+[a-z0-9]: \)\+[A-Z0-9][^:]\+[^:.]$'

count() {
	echo "$@" | grep -c "$travis_regex" || :
}

# Testing the regex itself.

#-------------------------------------------------------------------------------
# Correct patterns.

[[ $(count "foo, bar: Bar") -eq 1 ]]
[[ $(count "foo: Bar") -eq 1 ]]
[[ $(count "f1oo, b2ar: Bar") -eq 1 ]]
[[ $(count "2foo: Bar") -eq 1 ]]
[[ $(count "foo: bar: Barfoo") -eq 1 ]]
[[ $(count "foo: bar, foo: Barfoo") -eq 1 ]]
[[ $(count "foo: bar, foo: Barfoo") -eq 1 ]]
[[ $(count "resources: augeas: New resource") -eq 1 ]]
#===============================================================================

#-------------------------------------------------------------------------------
# Incorrect patterns.

# Space required after :
[[ $(count "foo:bar") -eq 0 ]]

# First char must be a a-z0-9
[[ $(count ", bar: bar") -eq 0 ]]

# Last chat before : must be a a-z0-9
[[ $(count "foo, : bar") -eq 0 ]]

# Last chat before : must be a a-z0-9
[[ $(count "foo,: bar") -eq 0 ]]

# No caps
[[ $(count "Foo: bar") -eq 0 ]]

# No dot at the end of the message.
[[ $(count "foo: bar.") -eq 0 ]]

# Capitalize the first word after :
[[ $(count "foo: bar") -eq 0 ]]

# More than one char is required before :
[[ $(count "a: bar") -eq 0 ]]

# Run checks agains multiple :.
[[ $(count "a: bar:") -eq 0 ]]
[[ $(count "a: bar, fooX: Barfoo") -eq 0 ]]
[[ $(count "a: bar, foo: barfoo foo: Nope") -eq 0 ]]
[[ $(count "nope a: bar, foo: barfoofoo: Nope") -eq 0 ]]
#===============================================================================

declare -i RC=0

test_commit_message() {
	declare -r long_hash="$1"

	declare short_hash="$(git show --pretty=%h --no-patch ${long_hash})"
	readonly short_hash

	declare summary="$(git show --pretty=%s --no-patch ${long_hash})"
	readonly summary

	info "Testing commit message ${long_hash}"

	if [[ $(count "${summary}") -eq 0 ]]; then
		err "Commit ${short_hash} does not conform to regex"
		indent "${summary}"
		RC=$(( RC | 1 ))
	fi

	if echo "${summary}" | grep "^resource:" &> /dev/null; then
		err "Commit ${short_hash} starts with \"resource:\"; did you mean \"resources:\"?"
		indent "${summary}"
		RC=$(( RC | 1 ))
	fi

	if echo "${summary}" | grep -q "^tests:"; then
		err "Commit ${short_hash} starts with \"tests:\", did you mean \"test:\"?"
		indent "${summary}"
		RC=$(( RC | 1 ))
	fi

	if echo "${summary}" | grep -q "^doc:"; then
		err "Commit ${short_hash} starts with \"doc:\", did you mean \"docs:\"?"
		indent "${summary}"
		RC=$(( RC | 1 ))
	fi

	if echo "${summary}" | grep -q "^example:"; then
		err "Commit ${short_hash} starts with \"example:\", did you mean \"examples:\"?"
		indent "${summary}"
		RC=$(( RC | 1 ))
	fi
}

commits=""
if [[ -n "${TRAVIS_PULL_REQUEST_SHA:-""}" ]]; then
	commits="$(git log --format=%H origin/"${TRAVIS_BRANCH}".."${TRAVIS_PULL_REQUEST_SHA}")"
	[[ -n "$commits" ]]
elif [[ -n "$(git show --pretty=%h --no-patch upstream/master)" ]]; then
	# If the user has run "misc/bootstrap.sh", upstream/master is a valid treeish.
	commits="$(git log --format=%H upstream/master..)"
else
	warn "Unable to find commit range; skipping commit messages."
fi

for commit in $commits; do
	test_commit_message $commit
done

if [[ ${RC} -ne 0 ]]; then
	err "Commit messages should match the following regex:"
	indent "${travis_regex}"
	echo
	indent "Examples:"
	indent "prometheus: Implement rest api"
	indent "resources: svc: Fix a race condition with reloads"
fi

exit ${RC}

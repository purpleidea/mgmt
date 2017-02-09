#!/bin/bash -e

travis_regex='^[a-z0-9]\(\(, \)\|[a-z0-9]\)\+[a-z0-9]: '

# Testing the regex itself.

# Correct patterns.
[[ $(echo "foo, bar: bar" | grep -c "$travis_regex") -eq 1 ]]
[[ $(echo "foo: bar" | grep -c "$travis_regex") -eq 1 ]]
[[ $(echo "f1oo, b2ar: bar" | grep -c "$travis_regex") -eq 1 ]]
[[ $(echo "2foo: bar" | grep -c "$travis_regex") -eq 1 ]]

# Space required after :
[[ $(echo "foo:bar" | grep -c "$travis_regex") -eq 0 ]]

# First char must be a a-z0-9
[[ $(echo ", bar: bar" | grep -c "$travis_regex") -eq 0 ]]

# Last chat before : must be a a-z0-9
[[ $(echo "foo, : bar" | grep -c "$travis_regex") -eq 0 ]]

# Last chat before : must be a a-z0-9
[[ $(echo "foo,: bar" | grep -c "$travis_regex") -eq 0 ]]

# No caps
[[ $(echo "Foo: bar" | grep -c "$travis_regex") -eq 0 ]]

# More than one char is required before :
[[ $(echo "a: bar" | grep -c "$travis_regex") -eq 0 ]]

test_commit_message() {
	echo Testing commit message $1
	if ! git log --format=%s $1 | head -n 1 | grep -q "$travis_regex"
	then
		echo "Commit message should follow the following regex: '$travis_regex'"
		echo
		echo "e.g:"
		echo "prometheus: implement rest api"
		echo "resources: svc: fix a race condition with reloads"
		exit 1
	fi
}

if [[ -n "$TRAVIS_PULL_REQUEST_SHA" ]]
then
	commits=$(git log --format=%H origin/${TRAVIS_BRANCH}..${TRAVIS_PULL_REQUEST_SHA})
	[[ -n "$commits" ]]

	for commit in $commits
	do
		test_commit_message $commit
	done
fi

#!/bin/bash -e
# tests if commit message conforms to convention

# library of utility functions
# shellcheck disable=SC1091
. test/util.sh

echo running "$0"

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}" || exit 1

commit_title_regex='^\([a-z0-9]\(\(, \)\|[a-z0-9]\)*[a-z0-9]: \)\+[A-Z0-9][^:]\+[^:.]$'

# Testing the regex itself.

# Correct patterns.
[[ $(echo "ci: Bar" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "foo, bar: Bar" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "foo: Bar" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "f1oo, b2ar: Bar" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "2foo: Bar" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "foo: bar: Barfoo" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "foo: bar, foo: Barfoo" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "foo: bar, foo: Barfoo" | grep -c "$commit_title_regex") -eq 1 ]]
[[ $(echo "resources: augeas: New resource" | grep -c "$commit_title_regex") -eq 1 ]]

# Space required after :
[[ $(echo "foo:bar" | grep -c "$commit_title_regex") -eq 0 ]]

# First char must be a a-z0-9
[[ $(echo ", bar: bar" | grep -c "$commit_title_regex") -eq 0 ]]

# Last char before : must be a a-z0-9
[[ $(echo "foo, : bar" | grep -c "$commit_title_regex") -eq 0 ]]

# Last char before : must be a a-z0-9
[[ $(echo "foo,: bar" | grep -c "$commit_title_regex") -eq 0 ]]

# No caps
[[ $(echo "Foo: bar" | grep -c "$commit_title_regex") -eq 0 ]]

# No dot at the end of the message.
[[ $(echo "foo: bar." | grep -c "$commit_title_regex") -eq 0 ]]

# Capitalize the first word after :
[[ $(echo "foo: bar" | grep -c "$commit_title_regex") -eq 0 ]]

# More than one char is required before :
[[ $(echo "a: bar" | grep -c "$commit_title_regex") -eq 0 ]]

# Run checks agains multiple :.
[[ $(echo "a: bar:" | grep -c "$commit_title_regex") -eq 0 ]]
[[ $(echo "a: bar, fooX: Barfoo" | grep -c "$commit_title_regex") -eq 0 ]]
[[ $(echo "a: bar, foo: barfoo foo: Nope" | grep -c "$commit_title_regex") -eq 0 ]]
[[ $(echo "nope a: bar, foo: barfoofoo: Nope" | grep -c "$commit_title_regex") -eq 0 ]]

test_commit_message() {
	echo "Testing commit message $1"
	title=$(git log --format=%s $1 | head -n 1)
	if ! echo "$title" | grep -q "$commit_title_regex"
	then
		echo "FAIL: Commit message should match the following regex: '$commit_title_regex'"
		echo "('$title' does not match)"
		echo
		echo "valid examples:"
		echo "prometheus: Implement rest api"
		echo "resources: svc: Fix a race condition with reloads"
		exit 1
	fi
}

test_commit_message_common_bugs() {
	echo "Testing commit message for common bugs $1"
	if git log --format=%s $1 | head -n 1 | grep -q "^resource:"
	then
		echo 'FAIL: Commit message starts with `resource:`, did you mean `engine: resources:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^resources:"
	then
		echo 'FAIL: Commit message starts with `resources:`, did you mean `engine: resources:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^engine: resource:"
	then
		echo 'FAIL: Commit message starts with `engine: resource:`, did you mean `engine: resources:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^tests:"
	then
		echo 'FAIL: Commit message starts with `tests:`, did you mean `test:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^doc:"
	then
		echo 'FAIL: Commit message starts with `doc:`, did you mean `docs:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^example:"
	then
		echo 'FAIL: Commit message starts with `example:`, did you mean `examples:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^language:"
	then
		echo 'FAIL: Commit message starts with `language:`, did you mean `lang:` ?'
		exit 1
	fi
	if git log --format=%s $1 | head -n 1 | grep -q "^lang: func:"
	then
		echo 'FAIL: Commit message starts with `lang: func:`, did you mean `lang: funcs:` ?'
		exit 1
	fi
}

test_commit_message_body() {
	echo "Testing commit message body $1"
	if git show --no-patch --format=%B $1 | grep -q "^Co-Authored-By:"
	then
		echo 'FAIL: Commit message includes `Co-Authored-By:`, did you mean `Co-authored-by:` ?'
		exit 1
	fi
	if git show --no-patch --format=%B $1 | grep -q "^Co-Authored-by:"
	then
		echo 'FAIL: Commit message includes `Co-Authored-by:`, did you mean `Co-authored-by:` ?'
		exit 1
	fi
	if git show --no-patch --format=%B $1 | grep -q "^co-authored-by:"
	then
		echo 'FAIL: Commit message includes `co-authored-by:`, did you mean `Co-authored-by:` ?'
		exit 1
	fi
	if git show --no-patch --format=%B $1 | grep -q "^Authored-By:"	# wat?
	then
		echo 'FAIL: Commit message includes `Authored-By:`, did you mean `Co-authored-by:` ?'
		exit 1
	fi

	if git show --no-patch --format=%B $1 | grep -q "^Signed-Off-By:"
	then
		echo 'FAIL: Commit message includes `Signed-Off-By:`, did you mean `Signed-off-by:` ?'
		exit 1
	fi
	if git show --no-patch --format=%B $1 | grep -q "^Signed-Off-by:"
	then
		echo 'FAIL: Commit message includes `Signed-Off-by:`, did you mean `Signed-off-by:` ?'
		exit 1
	fi
	if git show --no-patch --format=%B $1 | grep -q "^signed-off-by:"
	then
		echo 'FAIL: Commit message includes `signed-off-by:`, did you mean `Signed-off-by:` ?'
		exit 1
	fi
}

if [[ -n "$TRAVIS_PULL_REQUEST_SHA" ]]
then
	commits=$(git log --format=%H origin/${TRAVIS_BRANCH}..${TRAVIS_PULL_REQUEST_SHA})
elif [[ -n "$GITHUB_SHA" ]]
then
	# GITHUB_SHA is the HEAD of the branch
	# GITHUB_REF: The branch or tag ref that triggered the workflow. For example, refs/heads/feature-branch-1. If neither a branch or tag is available for the event type, the variable will not exist. (Not used.)
	# GITHUB_BASE_REF: Only set for pull request events. The name of the base branch.
	head=${GITHUB_SHA}
	if [[ -n "${GITHUB_BASE_REF}" ]]; then
		ref=${GITHUB_BASE_REF}
	else
		# this is just a push, no PR. Check commits outside origin/master
		ref=master
	fi
	commits=$(git log --no-merges --format=%H origin/${ref}..${head})
else # assume local branch
	commits=$(git log --no-merges --format=%H origin/master..HEAD)
fi

if [[ -n "$commits" ]]; then
	for commit in $commits
	do
		test_commit_message $commit
		test_commit_message_body $commit
		test_commit_message_common_bugs $commit
	done
fi

echo 'PASS'

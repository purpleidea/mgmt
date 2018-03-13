#!/bin/bash
set -eEu
set -o pipefail

################################################################################
# Tag and push a new release of the git repo.
# There are no arguments for this script.
################################################################################

# Variables:
#
# v: the old tag version, such as 0.0.13
# t: the new tag version, such as 0.0.14
# h: the head version, such as 0.0.13-40-g62ca126-dirty
v=$(git describe --match '[0-9]*\.[0-9]*\.[0-9]*' --tags --abbrev=0)
t=$(echo "${v%.*}.$((${v##*.}+1))")	# increment version
h="$(git describe --tags --dirty --always)"

# git remote to push the tag to ()
remote=${MGMT_TAG_REMOTE:-origin}

if [[ $# -gt 0 ]]; then
	echo "ERR: $0 does not take arguments." >&2
	exit 1
fi

# Never tag a dirty git tree.
if [[ "${h}" =~ dirty ]]; then
	echo "ERR: git tree is dirty. Commit or stash changes before tagging." >&2
	exit 1
fi

# Be idempotent.
if [[ "${h}" == "${v}" ]]; then
	echo "INFO: HEAD \"${h}\" is equivalent to the current (old) version tag \"${v}\"; nothing to do."
	exit 0
fi

# Give the user a chance to abort.
echo "WARN: About to tag \"${h}\" as \"${t}\" and push."
echo "Press ^C within 3s to abort."
sleep 3s

# Make releasenotes
releasenotes=$(mktemp)
echo "release: tag $t" > "$releasenotes"
# create temporary tag in order for changelog to render last release properly
git tag "$t"
misc/changelog-from-git.sh >> "$releasenotes"

# Tag and push.
# annotate tag and force to overwrite previous temporary tag
git tag -a -f --file=- --sign "$t" < "$releasenotes"
echo "INFO: Version $t is now tagged."
echo "INFO: Pushing $t to origin."
git push "$remote" "$t"

# Be informative.
env GIT_PAGER=cat git diff --stat "$v" "$t"
if command -v contrib.sh &>/dev/null; then contrib.sh "$v"; fi
echo -e "run 'git log $v..$t' to see what has changed since $v"

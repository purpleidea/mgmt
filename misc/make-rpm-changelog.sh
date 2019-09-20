#!/bin/bash
# This script generates an rpm changelog from the project's git history.

# version we're releasing
DISTRO="$1"
VERSION="$2"
if [ "$VERSION" = "" ]; then
	echo "usage: ./$0 <distro> <version>"
	exit 1
fi
# path to store the changelog
CHANGELOG="releases/${VERSION}/${DISTRO}/changelog"
dir="$(dirname "$CHANGELOG")/"
if [ ! -d "$dir" ]; then
	echo "changelog dir ($dir) does not exist"
	exit 1
fi
# input to format flag for git tag
TAG_FORMAT="* %(creatordate:format:%a %b %d %Y) %(creator) %(refname:lstrip=2)"
# a list of tags to be parsed in the loop
TAGS=$(git tag --sort=-creatordate --format="$TAG_FORMAT" | sed -r 's/[0-9]+ -[0-9]+ //')

# placeholder for the next line of the list
THIS_TAGLINE=

# parse the list
while read -r LAST_TAGLINE; do
	# read ahead one tag
	if [ "$THIS_TAGLINE" == "" ]; then
		# store the tag for the next iteration
		THIS_TAGLINE="$LAST_TAGLINE"
		continue
	fi

	# grab the tags from the last column of the taglines
	THIS_TAG=$(echo "$THIS_TAGLINE" | awk '{print $NF}')
	LAST_TAG=$(echo "$LAST_TAGLINE" | awk '{print $NF}')

	# print the release description
	printf "\n%s\n\n" "$THIS_TAGLINE" >> "$CHANGELOG"

	# print all the commits between the tags
	git shortlog -n ${LAST_TAG}...${THIS_TAG} | sed -r '/\):/s/^/ - /' >> "$CHANGELOG"

	# first tag is special since there's no previous one
	if [ "$LAST_TAG" == "0.0.1" ]; then
		# print the release description
		printf "\n%s\n\n" "$LAST_TAGLINE" >> "$CHANGELOG"
		# print all the commits before the first tag
		git shortlog -n $LAST_TAG | sed -r '/\):/s/^/ - /' >> "$CHANGELOG"
	fi

	# store the tag for the next iteration
	THIS_TAGLINE="$LAST_TAGLINE"
done <<< "$TAGS"

# trim the first and last lines
sed -i '1d;$d' "$CHANGELOG"

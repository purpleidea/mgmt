#!/bin/bash
# This script packages rpm, deb, and pacman packages of mgmt with fpm. The
# first argument is the package type, and all subsequent arguments are the
# dependencies. Example usage: `./fpm-pack.sh deb dependency1 dependency2`

# the binary to package
BINARY="mgmt"
# git tag pointing to the current commit
TAG=$(git tag -l --points-at HEAD)
# maintainer email
MAINTAINER="mgmtconfig@purpleidea.com"
# project url
URL="https://github.com/purpleidea/mgmt/"
# project description
DESCRIPTION="Next generation distributed, event-driven, parallel config management!"
# project license
LICENSE="GPLv3"
# location to install the binary
PREFIX="/usr/bin"
# release directory
DIR="releases"

# placeholder for dependencies to be read from arguments
DEPS=
# placeholder for changelog argument parsed from the package type
CHANGELOG=

# make sure we're on a tagged commit
if [ "$TAG" == "" ]; then
	echo "cannot release an untagged commit"
	exit 1
fi

if [ "$2" == "" ]; then
	echo "version was not specified"
	exit 1
fi
VERSION="$2"

if [ "$VERSION" != "$TAG" ]; then
	echo "you must checkout the correct version before building (${VERSION} != ${TAG})"
	exit 1
fi

# make sure the package type is valid
if [ "$1" != "deb" ] && [ "$1" != "rpm" ] && [ "$1" != "pacman" ]; then
	echo "invalid package type"
	exit 1
fi

# there are no changelogs for pacman packages
if [ "$1" != "pacman" ]; then
	CHANGELOG="--${1}-changelog=${DIR}/${VERSION}/${1}/changelog"
fi

# arguments after the first two are deps
for i in "${@:3}"; do
	DEPS="$DEPS -d $i"
done

# build the package
fpm \
	--log error \
	--name "$BINARY" \
	--version "$TAG" \
	--maintainer "$MAINTAINER" \
	--url "$URL" \
	--description "$DESCRIPTION" \
	--license "$LICENSE" \
	--input-type dir \
	--output-type "$1" \
	--package "${DIR}/${VERSION}/${1}/" \
	${CHANGELOG} \
	${DEPS} \
	--prefix "$PREFIX" \
	"$BINARY"

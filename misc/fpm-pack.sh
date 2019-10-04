#!/bin/bash
# This script packages rpm, deb, and pacman packages of mgmt with fpm. The
# first argument is the distro type, and the second argument is the version. All
# subsequent arguments are the dependencies.
# Example usage: `./fpm-pack.sh fedora-29 0.1.2 dependency1 dependency2`

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

DISTRO="$1"
if [ "$1" == "" ]; then
	echo "distro was not specified"
	exit 1
fi
VERSION="$2"
if [ "$VERSION" == "" ]; then
	echo "version was not specified"
	exit 1
fi
OUTPUT="$3"
if [ "$OUTPUT" == "" ]; then
	echo "output file was not specified"
	exit 1
fi

if [ "$VERSION" != "$TAG" ]; then
	echo "you must checkout the correct version before building (${VERSION} != ${TAG})"
	exit 1
fi

# make sure the distro is a known valid one
if [[ "$DISTRO" == fedora-* ]]; then
	typ="rpm"
elif [[ "$DISTRO" == centos-* ]]; then
	typ="rpm"
elif [[ "$DISTRO" == debian-* ]]; then
	typ="deb"
elif [[ "$DISTRO" == ubuntu-* ]]; then
	typ="deb"
elif [[ "$DISTRO" == archlinux ]]; then
	typ="pacman"
else
	echo "unknown distro: ${DISTRO}."
	exit 1
fi

if [ "$typ" != "rpm" ] && [ "$typ" != "deb" ] && [ "$typ" != "pacman" ]; then
	echo "invalid package type"
	exit 1
fi

# assume the file extension
ext="$typ"
if [ "$typ" = "pacman" ]; then	# archlinux is an exception
	ext="pkg.tar.xz"
fi

# don't run if the file already exists (bad idempotent implementation)
if [ -d "${DIR}/${VERSION}/${DISTRO}/" ]; then
	if ls "${DIR}/${VERSION}/${DISTRO}/"*."${ext}" &>/dev/null; then
		# update timestamp so the Makefile is happy =D
		touch "${DIR}/${VERSION}/${DISTRO}/"*."${ext}"
		echo "a .${ext} already exists"
		exit 0	# don't error, we want to be idempotent
	fi
fi

# there are no changelogs for pacman packages
if [ "$typ" != "pacman" ]; then
	CHANGELOG="--${typ}-changelog=${DIR}/${VERSION}/${DISTRO}/changelog"
fi

# arguments after the first three are deps
for i in "${@:4}"; do
	DEPS="$DEPS --depends $i"
done

# in case the `fpm` gem bin isn't in the $PATH
if command -v ruby >/dev/null && command -v gem >/dev/null && ! command -v fpm 2>/dev/null; then
	PATH="$(ruby -r rubygems -e 'puts Gem.user_dir')/bin:$PATH"
fi

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
	--output-type "$typ" \
	--package "${DIR}/${VERSION}/${DISTRO}/${OUTPUT}" \
	${CHANGELOG} \
	${DEPS} \
	--prefix "$PREFIX" \
	"$BINARY"

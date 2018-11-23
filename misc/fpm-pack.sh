#!/bin/bash

BINARY="mgmt"
MAINTAINER="mgmt@noreply.github.com"
URL="https://github.com/purpleidea/mgmt"
DESCRIPTION="Next generation distributed, event-driven, parallel config management!"
LICENSE="GPLv3"
PREFIX="/usr/bin"
TAG=$(git tag -l --points-at HEAD)

# construct fpm command with deps($1) and package type($2) as arguments
function pack {
	fpm -n  "$BINARY" -v "$TAG" -m "$MAINTAINER" --url "$URL" \
		--description "$DESCRIPTION" --license "$LICENSE" $1 \
		-s dir -t $2 --log error --prefix "$PREFIX" "$BINARY"
}

# build the DEB package
pack "-d libvirt-dev -d libaugeas-dev" "deb"
# build the RPM package
pack "-d libvirt-devel -d libaugeas-devel" "rpm"
# build the PKG package
pack "-d libvirt -d augeas" "pacman"

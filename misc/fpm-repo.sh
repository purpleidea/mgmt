#!/usr/bin/env bash
# This script makes packages of mgmt using fpm. It pulls in any binaries from:
# releases/$version/binary-linux-$arch/mgmt-linux-$arch-$version and builds to:
# repository/$distro-$version/$arch/ before it runs the appropriate createrepo
# commands.

# NOTE: run `gem install fpm` to update my ~/bin/fpm to the latest version.
# TODO: consider switching to https://github.com/goreleaser/nfpm

# the binary to package
BINARY="mgmt"
# maintainer email
MAINTAINER="fpm@mgmtconfig.com"
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
# repository directory
OUT="repository"

## make sure the distro is a known valid one
#if [[ "$DISTRO" == fedora-* ]]; then
#	typ="rpm"
#elif [[ "$DISTRO" == centos-* ]]; then
#	typ="rpm"
#elif [[ "$DISTRO" == debian-* ]]; then
#	typ="deb"
#elif [[ "$DISTRO" == ubuntu-* ]]; then
#	typ="deb"
#elif [[ "$DISTRO" == archlinux ]]; then
#	typ="pacman"
#else
#	echo "unknown distro: ${DISTRO}."
#	exit 1
#fi

#if [ "$typ" != "rpm" ] && [ "$typ" != "deb" ] && [ "$typ" != "pacman" ]; then
#	echo "invalid package type"
#	exit 1
#fi

## assume the file extension
#ext="$typ"
#if [ "$typ" = "pacman" ]; then	# archlinux is an exception
#	ext="pkg.tar.xz"
#fi

# in case the `fpm` gem bin isn't in the $PATH
if command -v ruby >/dev/null && command -v gem >/dev/null && ! command -v fpm >/dev/null; then
	PATH="$(ruby -r rubygems -e 'puts Gem.user_dir')/bin:$PATH"
fi

# skip putting these versions into the repos
skip_mgmt_version=()
skip_mgmt_version+=("0.0.25")
skip_mgmt_version+=("0.0.26")
skip_mgmt_version+=("0.0.27")

# from binary arch to repoarch
declare -A map_repoarch=(
	[amd64]="x86_64"
	[arm64]="aarch64"
)

declare -A map_distrotype=(
	[fedora]="rpm"
	[debian]="deb"
)

declare -A map_distro_version=(
	["fedora-41"]="libvirt-devel augeas-devel"
	["fedora-42"]="libvirt-devel augeas-devel"
	["debian-13"]="libvirt-dev libaugeas-dev"
)

#echo releases:
#for dv in "fedora-41" "fedora-42" "debian-11" "archlinux-xx"; do
for dv in "${!map_distro_version[@]}"; do
	distro=${dv%%-*};
	version=${dv##*-}
	deps=${map_distro_version[$dv]}

	echo "distro-version: ${distro}-${version}"
	mkdir -p ${OUT}/$dv/

	type=${map_distrotype[$distro]}

	# track the arches we see
	declare -A repoarches=()

	for chunk1 in ${DIR}/*; do
		if [ ! -d "$chunk1" ]; then # check if it's a regular dir
			continue
		fi
		package_version=$(basename "$chunk1")
		#echo "package_version: $package_version"

		if [[ " ${skip_mgmt_version[*]} " == *" $package_version "* ]]; then
			echo "skip: ${package_version}"
			continue
		fi

		for chunk2 in $chunk1/binary-linux-*; do
			if [ ! -d "$chunk2" ]; then # check if it's a regular dir
				continue
			fi
			arch=${chunk2##*-}
			#echo "arch: $arch"

			repoarch=${map_repoarch[$arch]}
			#echo "repoarch: $repoarch"

			repoarches["${repoarch}"]="${type}" # tag it

			mkdir -p ${OUT}/${distro}-${version}/$repoarch/

			#file $chunk2/mgmt-linux-$arch-$package_version # found it

			output="${OUT}/${distro}-${version}/${repoarch}/mgmt-${package_version}.${repoarch}.${type}"

			if [ -f "$output" ]; then
				echo "skip: ${output}"
				continue
			fi

			depends=""
			for i in $deps; do
				depends="$depends --depends $i"
			done

			# build the package
			echo "fpm..."
			echo "output: ${output}"
			fpm \
			--log error \
			--name "$BINARY" \
			--version "$package_version" \
			--maintainer "$MAINTAINER" \
			--url "$URL" \
			--description "$DESCRIPTION" \
			--license "$LICENSE" \
			--input-type dir \
			--output-type "$type" \
			--package "${output}" \
			${depends} \
			"misc/mgmt.service"="/usr/lib/systemd/system/mgmt.service" \
			"$BINARY"="$PREFIX/mgmt" \
			|| rm "${output}" # if it fails, remove it...
		done
	done

	# now run createrepo or similar
	for key in "${!repoarches[@]}"; do
		type=${repoarches[$key]}
		outdir="${OUT}/$dv/${key}/"
		if [ "$type" = "rpm" ]; then
			echo "createrepo ${type} ${outdir}"
			# TODO: use --deltas ?
			createrepo_c --update "${outdir}"
		fi
		if [ "$type" = "deb" ]; then
			cd ${outdir} > /dev/null
			# don't regenerate unnecessarily
			if [[ ! -f Packages.gz ]] || find . -name '*.deb' -newer Packages.gz | grep -q .; then
				echo "dpkg-scanpackages ${type} ${outdir}"
				dpkg-scanpackages --multiversion . /dev/null | gzip -9 > Packages.gz
			fi
			cd - > /dev/null # silence it
		fi
	done
done

USERNAME=$(cat ~/.config/copr 2>/dev/null | grep username | awk -F '=' '{print $2}' | tr -d ' ')
SERVER='dl.fedoraproject.org'
REMOTE_PATH="/srv/pub/alt/${USERNAME}/${BINARY}/repo/"
if [ "${USERNAME}" = "" ]; then
	echo "empty username, can't rsync"
fi

rsync -avzSH --progress --delete ${OUT}/ ${SERVER}:${REMOTE_PATH}

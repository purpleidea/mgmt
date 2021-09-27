#!/bin/bash
# setup a simple golang environment
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

. ${ROOT}/test/util.sh

sudo_command=$(command -v sudo)

GO=`command -v go 2>/dev/null`
YUM=`command -v yum 2>/dev/null`
DNF=`command -v dnf 2>/dev/null`
APT=`command -v apt-get 2>/dev/null`
NEWAPT=`command -v apt 2>/dev/null`
BREW=`command -v brew 2>/dev/null`
PACMAN=`command -v pacman 2>/dev/null`

# set minimum golang version and installed golang version
mingolangversion=16
golangversion=0
if [ -x "$GO" ]; then
	# capture the minor version number
	golangversion=$(go version | grep -o -P '(?<=go1\.)[0-9]*')
fi

# if DNF is available use it
if [ -x "$DNF" ]; then
	YUM=$DNF
fi

# if APT is available use it
if [ -x "$NEWAPT" ]; then
	APT=$NEWAPT
fi

if [ -z "$YUM" -a -z "$APT" -a -z "$BREW" -a -z "$PACMAN" ]; then
	echo "The package managers can't be found."
	exit 1
fi

# I think having both installed confused golang somehow...
if [ -n "$YUM" -a -n "$APT" ]; then
	echo "You have both $APT and $YUM installed. Please check your deps manually."
fi

fold_start "Install dependencies"
if [ -n "$YUM" ]; then
	$sudo_command $YUM install -y libvirt-devel
	$sudo_command $YUM install -y augeas-devel
	$sudo_command $YUM install -y ruby-devel rubygems
	$sudo_command $YUM install -y time
	$sudo_command $YUM install -y ragel
	# dependencies for building packages with fpm
	$sudo_command $YUM install -y gcc make rpm-build libffi-devel bsdtar mkosi || true
	$sudo_command $YUM install -y graphviz || true # for debugging
fi
if [ -n "$APT" ]; then
	$sudo_command $APT update -y
	$sudo_command $APT install -y libvirt-dev || true
	$sudo_command $APT install -y libaugeas-dev || true
	$sudo_command $APT install -y ruby ruby-dev || true
	$sudo_command $APT install -y libpcap0.8-dev || true
	$sudo_command $APT install -y ragel || true
	# dependencies for building packages with fpm
	$sudo_command $APT install -y build-essential rpm bsdtar || true
	# `realpath` is a more universal alternative to `readlink -f` for absolute path resolution
	# (-f is missing on BSD/macOS), but older Debian/Ubuntu's don't include it in coreutils yet.
	# https://unix.stackexchange.com/a/136527
	$sudo_command $APT install -y realpath || true
	$sudo_command $APT install -y time || true
	$sudo_command $APT install -y inotify-tools # used by some tests
	$sudo_command $APT install -y graphviz # for debugging
fi

# Prevent linuxbrew installing redundant deps in CI
if [ -n "$BREW" -a "$RUNNER_OS" != "Linux" ]; then
	# coreutils contains gtimeout, gstat, etc
	$BREW install pkg-config libvirt augeas coreutils ragel || true
fi

if [ -n "$PACMAN" ]; then
	$sudo_command $PACMAN -S --noconfirm --asdeps --needed libvirt augeas rubygems libpcap ragel
fi
fold_end "Install dependencies"

if ! in_ci; then
	if [ -n "$YUM" ]; then
		if [ -z "$GO" ]; then
			$sudo_command $YUM install -y golang golang-googlecode-tools-stringer || $sudo_command $YUM install -y golang-bin # centos-7 epel
		fi
		# some golang dependencies are stored in mercurial
		$sudo_command $YUM install -y hg
	fi
	if [ -n "$APT" ]; then
		$sudo_command $APT update
		if [ -z "$GO" ]; then
			$sudo_command $APT install -y golang
			# one of these two golang tools packages should work on debian
			$sudo_command $APT install -y golang-golang-x-tools || true
			$sudo_command $APT install -y golang-go.tools || true
		fi
		$sudo_command $APT install -y build-essential packagekit mercurial
	fi
	if [ -n "$PACMAN" ]; then
		$sudo_command $PACMAN -S --noconfirm --asdeps --needed go gcc pkg-config
	fi
fi

if in_ci; then
	# TODO: consider bumping to new package manager version
	RAGEL_VERSION='6.10'	# current stable version
	RAGEL_TMP='/tmp/ragel/'
	RAGEL_FILE="${RAGEL_TMP}ragel-${RAGEL_VERSION}.tar.gz"
	RAGEL_DIR="${RAGEL_TMP}ragel-${RAGEL_VERSION}/"
	mkdir -p "$RAGEL_TMP"
	cd "$RAGEL_TMP"
	wget "https://www.colm.net/files/ragel/ragel-${RAGEL_VERSION}.tar.gz" -O "$RAGEL_FILE"
	tar -xvf "$RAGEL_FILE"
	cd -
	cd "$RAGEL_DIR"
	./configure --prefix=/usr/local --disable-manual
	make
	sudo make install
	cd -
fi

# attempt to workaround old ubuntu
if [ -n "$APT" -a "$golangversion" -lt "$mingolangversion" ]; then
	echo "install golang from a ppa."
	$sudo_command $APT remove -y golang
	$sudo_command $APT install -y software-properties-common	# for add-apt-repository
	$sudo_command add-apt-repository -y ppa:longsleep/golang-backports
	$sudo_command $APT update -y
	$sudo_command $APT install -y golang-go
fi

# if golang is too old, we don't want to fail with an obscure error later
if [ "$golangversion" -lt "$mingolangversion" ]; then
	echo "mgmt recommends go1.$mingolangversion or higher."
	exit 1
fi

[ -e "$GOBIN/mgmt" ] && rm -f "$GOBIN/mgmt"	# the `go get` version has no -X

fold_start "Install golang tools"
# TODO: change this for golang 1.17
go get github.com/blynn/nex				# for lexing
go get golang.org/x/tools/cmd/goyacc			# formerly `go tool yacc`
go get golang.org/x/tools/cmd/stringer			# for automatic stringer-ing
go get golang.org/x/lint/golint				# for `golint`-ing
go get golang.org/x/tools/cmd/goimports		# for fmt
go get github.com/kevinburke/go-bindata/go-bindata	# for compiling in non golang files
go get github.com/dvyukov/go-fuzz/go-fuzz		# for fuzzing the mcl lang bits
if in_ci; then
	go get -u gopkg.in/alecthomas/gometalinter.v1 && \
	mv "$(dirname $(command -v gometalinter.v1))/gometalinter.v1" "$(dirname $(command -v gometalinter.v1))/gometalinter" && \
	gometalinter --install	# bonus
fi
fold_end "Install golang tools"

fold_start "Install miscellaneous tools"
command -v mdl &>/dev/null || gem install mdl --no-document || true	# for linting markdown files
command -v fpm &>/dev/null || gem install fpm --no-document || true	# for cross distro packaging
fold_end "Install miscellaneous tools"

cd "$XPWD" >/dev/null

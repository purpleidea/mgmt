#!/bin/bash
# setup a simple go environment
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

sudo_command=$(command -v sudo)

GO=`command -v go 2>/dev/null`
YUM=`command -v yum 2>/dev/null`
DNF=`command -v dnf 2>/dev/null`
APT=`command -v apt-get 2>/dev/null`
BREW=`command -v brew 2>/dev/null`
PACMAN=`command -v pacman 2>/dev/null`

# if DNF is available use it
if [ -x "$DNF" ]; then
	YUM=$DNF
fi

if [ -z "$YUM" -a -z "$APT" -a -z "$BREW" -a -z "$PACMAN" ]; then
	echo "The package managers can't be found."
	exit 1
fi

if [ ! -z "$YUM" ]; then
	$sudo_command $YUM install -y libvirt-devel
	$sudo_command $YUM install -y augeas-devel
	$sudo_command $YUM install -y time
fi
if [ ! -z "$APT" ]; then
	$sudo_command $APT install -y libvirt-dev || true
	$sudo_command $APT install -y libaugeas-dev || true
	$sudo_command $APT install -y libpcap0.8-dev || true
	# dependencies for building debian packages with `make deb`
	$sudo_command $APT install -y dpkg-dev devscripts debhelper dh-golang dh-systemd
	# `realpath` is a more universal alternative to `readlink -f` for absolute path resolution
	# (-f is missing on BSD/macOS), but older Debian/Ubuntu's don't include it in coreutils yet.
	# https://unix.stackexchange.com/a/136527
	$sudo_command $APT install -y realpath || true
	$sudo_command $APT install -y time || true
fi

if [ ! -z "$BREW" ]; then
	# coreutils contains gtimeout, gstat, etc
	$BREW install libvirt augeas coreutils || true
fi

if [ ! -z "$PACMAN" ]; then
	$sudo_command $PACMAN -S --noconfirm --asdeps --needed libvirt augeas libpcap
fi

if [ $travis -eq 0 ]; then
	if [ ! -z "$YUM" ]; then
		if [ -z "$GO" ]; then
			$sudo_command $YUM install -y golang golang-googlecode-tools-stringer
		fi
		# some go dependencies are stored in mercurial
		$sudo_command $YUM install -y hg
	fi
	if [ ! -z "$APT" ]; then
		$sudo_command $APT update
		if [ -z "$GO" ]; then
			$sudo_command $APT install -y golang
			# one of these two golang tools packages should work on debian
			$sudo_command $APT install -y golang-golang-x-tools || true
			$sudo_command $APT install -y golang-go.tools || true
		fi
		$sudo_command $APT install -y build-essential packagekit mercurial
	fi
	if [ ! -z "$PACMAN" ]; then
		$sudo_command $PACMAN -S --noconfirm --asdeps --needed go gcc pkg-config
	fi
fi

# if golang is too old, we don't want to fail with an obscure error later
if go version | grep 'go1\.[012345]\.'; then
	echo "mgmt requires go1.6 or higher."
	exit 1
fi

echo "running 'go get -v -d ./...' from `pwd`"
go get -v -t -d ./...	# get all the go dependencies
echo "done running 'go get -v -t -d ./...'"

[ -e "$GOBIN/mgmt" ] && rm -f "$GOBIN/mgmt"	# the `go get` version has no -X
# vet is built-in in go 1.6 - we check for go vet command
go vet 1> /dev/null 2>&1
if [[ $? != 0 ]]; then
	go get golang.org/x/tools/cmd/vet      # add in `go vet` for travis
fi
go get github.com/blynn/nex				# for lexing
go get golang.org/x/tools/cmd/goyacc			# formerly `go tool yacc`
go get golang.org/x/tools/cmd/stringer			# for automatic stringer-ing
go get golang.org/x/lint/golint				# for `golint`-ing
go get github.com/tmthrgd/go-bindata/go-bindata	# for compiling in non golang files
go get -u gopkg.in/alecthomas/gometalinter.v1 && mv "$(dirname $(command -v gometalinter.v1))/gometalinter.v1" "$(dirname $(command -v gometalinter.v1))/gometalinter" && gometalinter --install	# bonus
command -v mdl &>/dev/null || gem install mdl || true	# for linting markdown files
cd "$XPWD" >/dev/null

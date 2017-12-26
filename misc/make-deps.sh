#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

# setup a simple go environment
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

sudo_command="$(command -v sudo || :)"

YUM="$(command -v yum || :)"
DNF="$(command -v dnf || :)"
APT="$(command -v apt-get || :)"
BREW="$(command -v brew || :)"
PACMAN="$(command -v pacman || :)"

# if DNF is available use it
if [ -x "$DNF" ]; then
	YUM=$DNF
fi

if [[ -z "$YUM" ]] && [[ -z "$APT" ]] && [[ -z "$BREW" ]] && [[ -z "$PACMAN" ]]; then
	err "The package managers can't be found."
	exit 1
fi

if [[ -n "$YUM" ]]; then
	$sudo_command $YUM install -y libvirt-devel
	$sudo_command $YUM install -y augeas-devel
fi

if [[ -n "$APT" ]]; then
	$sudo_command $APT install -y libvirt-dev || true
	$sudo_command $APT install -y libaugeas-dev || true
	$sudo_command $APT install -y libpcap0.8-dev || true
fi

if [[ -n "$BREW" ]]; then
	$BREW install libvirt || true
fi

if [[ -n "$PACMAN" ]]; then
	$sudo_command $PACMAN -S --noconfirm libvirt augeas libpcap
fi

if [ $travis -eq 0 ]; then
	if [[ -n "$YUM" ]]; then
		# some go dependencies are stored in mercurial
		$sudo_command $YUM install -y golang golang-googlecode-tools-stringer hg
	fi

	if [[ -n "$APT" ]]; then
		$sudo_command $APT update
		$sudo_command $APT install -y golang make gcc packagekit mercurial
		# one of these two golang tools packages should work on debian
		$sudo_command $APT install -y golang-golang-x-tools || true
		$sudo_command $APT install -y golang-go.tools || true
	fi

	if [[ -n "$PACMAN" ]]; then
		$sudo_command $PACMAN -S --noconfirm go
	fi
fi

# if golang is too old, we don't want to fail with an obscure error later
if go version | grep 'go1\.[012345]\.'; then
	echo "mgmt requires go1.6 or higher."
	exit 1
fi

go get -d ./...	# get all the go dependencies
[ -e "$GOBIN/mgmt" ] && rm -f "$GOBIN/mgmt"	# the `go get` version has no -X
# vet is built-in in go 1.6 - we check for go vet command
go vet 1> /dev/null 2>&1
ret=$?
if [[ $ret != 0 ]]; then
	go get golang.org/x/tools/cmd/vet      # add in `go vet` for travis
fi
go get golang.org/x/tools/cmd/stringer			# for automatic stringer-ing
go get github.com/jteeuwen/go-bindata/go-bindata	# for compiling in non golang files
go get github.com/golang/lint/golint			# for `golint`-ing
go get -u gopkg.in/alecthomas/gometalinter.v1 && mv "$(dirname $(which gometalinter.v1))/gometalinter.v1" "$(dirname $(which gometalinter.v1))/gometalinter" && gometalinter --install	# bonus
cd "$XPWD" >/dev/null

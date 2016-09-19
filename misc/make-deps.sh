#!/usr/bin/env bash
# setup a simple go environment
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

sudo_command=$(which sudo)

if [ $travis -eq 0 ]; then
	YUM=`which yum 2>/dev/null`
	APT=`which apt-get 2>/dev/null`
	if [ -z "$YUM" -a -z "$APT" ]; then
		echo "The package managers can't be found."
		exit 1
	fi
	if [ ! -z "$YUM" ]; then
		# some go dependencies are stored in mercurial
		$sudo_command $YUM install -y golang golang-googlecode-tools-stringer hg

	fi
	if [ ! -z "$APT" ]; then
		$sudo_command $APT update
		$sudo_command $APT install -y golang make gcc packagekit mercurial
		# one of these two golang tools packages should work on debian
		$sudo_command $APT install -y golang-golang-x-tools || true
		$sudo_command $APT install -y golang-go.tools || true
		$sudo_command $APT install -y libpcap0.8-dev || true
	fi
fi

# if golang is too old, we don't want to fail with an obscure error later
if go version | grep 'go1\.[0123]\.'; then
	echo "mgmt requires go1.4 or higher."
	exit 1
fi

go get ./...	# get all the go dependencies
[ -e "$GOBIN/mgmt" ] && rm -f "$GOBIN/mgmt"	# the `go get` version has no -X
# vet is built-in in go 1.6 - we check for go vet command
go vet 1> /dev/null 2>&1
ret=$?
if [[ $ret != 0 ]]; then
	go get golang.org/x/tools/cmd/vet      # add in `go vet` for travis
fi
go get golang.org/x/tools/cmd/stringer	# for automatic stringer-ing
go get github.com/golang/lint/golint	# for `golint`-ing
cd "$XPWD" >/dev/null

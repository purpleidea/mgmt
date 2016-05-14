#!/bin/bash
# setup a simple go environment
XPWD=`pwd`
ROOT="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && cd .. && pwd )"	# dir!
cd "${ROOT}" >/dev/null

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

if [ $travis -eq 0 ]; then
	YUM=`which yum 2>/dev/null`
	APT=`which apt-get 2>/dev/null`
	if [ -z "$YUM" -a -z "$APT" ]; then
		echo "The package managers can't be found."
		exit 1
	fi
	if [ ! -z "$YUM" ]; then
		# some go dependencies are stored in mercurial
		sudo $YUM install -y golang golang-googlecode-tools-stringer hg

	fi
	if [ ! -z "$APT" ]; then
		sudo $APT update
		sudo $APT install -y golang make gcc packagekit mercurial
		# one of these two golang tools packages should work on debian
		sudo $APT install -y golang-golang-x-tools || true
		sudo $APT install -y golang-go.tools || true
	fi
fi

# build etcd
git clone --recursive https://github.com/coreos/etcd/ && cd etcd
goversion=$(go version)
# if 'go version' contains string 'devel', then use git master of etcd...
if [ "${goversion#*devel}" == "$goversion" ]; then
	git checkout v2.2.4	# TODO: update to newer versions as needed
fi
[ -x build ] && ./build
mkdir -p ~/bin/
cp bin/etcd ~/bin/
cd - >/dev/null
rm -rf etcd	# clean up to avoid failing on upstream gofmt errors

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

#!/bin/bash
# setup a simple go environment

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
		sudo $APT install -y golang golang-golang-x-tools mercurial

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
cd -
rm -rf etcd	# clean up to avoid failing on upstream gofmt errors

go get ./...	# get all the go dependencies
[ -e "$GOBIN/mgmt" ] && rm -f "$GOBIN/mgmt"	# the `go get` version has no -X
go get golang.org/x/tools/cmd/vet	# add in `go vet` for travis
go get golang.org/x/tools/cmd/stringer	# for automatic stringer-ing

#!/bin/bash
# setup a simple go environment

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

if [ $travis -eq 0 ]; then
	YUM=`which yum`
	if [ -z $YUM ]; then
		echo "The 'yum' utility can't be found."
		exit 1
	fi
	sudo yum install -y golang golang-googlecode-tools-stringer
	sudo yum install -y hg	# some go dependencies are stored in mercurial
fi

# build etcd
git clone --recursive https://github.com/coreos/etcd/ && cd etcd
git checkout v2.2.4	# TODO: update to newer versions as needed
[ -x build ] && ./build
mkdir -p ~/bin/
cp bin/etcd ~/bin/
cd -
rm -rf etcd	# clean up to avoid failing on upstream gofmt errors

go get ./...	# get all the go dependencies
go get golang.org/x/tools/cmd/vet # add in `go vet` for travis
go get golang.org/x/tools/cmd/stringer	# for automatic stringer-ing

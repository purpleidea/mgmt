#!/bin/bash
# setup a simple go environment

# see if we're running a testrun in travis
travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

# check the builtin $OSTYPE variable to see which OS we're running
case "$OSTYPE" in
	solaris*) os='solaris' ;;
	darwin*)  os='osx' ;;
	linux*)   os='linux' ;;
	bsd*)     os='bsd' ;;
	*)        echo "unknown: $OSTYPE"; exit 1 ;;
esac

if [ "$os" == "linux" ]; then
	# if we're not doing a travis run we need to install some prereqs
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
	git checkout v2.2.4 # TODO: update to newer versions as needed
	[ -x build ] && ./build
	mkdir -p ~/bin/
	cp bin/etcd ~/bin/
	cd -
	rm -rf etcd # clean up to avoid failing on upstream gofmt errors

fi

if [ "$os" = "osx" ]; then
	BREW=`which brew`
	if [ -z $BREW ]; then
		echo "The 'brew' utility can't be found."
		exit 1
	fi
	brew install -y go etcd
fi


go get ./...	# get all the go dependencies
go get golang.org/x/tools/cmd/vet # add in `go vet` for travis
go get golang.org/x/tools/cmd/stringer	# for automatic stringer-ing

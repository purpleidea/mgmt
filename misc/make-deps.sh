#!/bin/bash
# setup a simple go environment

travis=0
if env | grep -q '^TRAVIS=true$'; then
	travis=1
fi

if [ $travis -eq 0 ]; then
	YUM=`which yum`
	if [ -z $YUM ]; then
		echo "The 'yum' utility can't be installed."
		exit 1
	fi
	sudo yum install -y golang
	sudo yum install -y hg	# some go dependencies are stored in mercurial
fi

if ! env | grep -q '^GOPATH='; then
	export GOPATH="$HOME/gopath/"
	mkdir "$GOPATH"
	if ! grep -F '^export GOPATH=' ~/.bashrc; then
		echo "export GOPATH=~/gopath/" >> ~/.bashrc
	fi
	echo "setting go path to: $GOPATH"
fi

echo "gopath is: $GOPATH"

go get ./...	# get all the go dependencies

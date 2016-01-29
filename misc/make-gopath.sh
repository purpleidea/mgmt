#!/bin/bash
# setup a simple go environment

if ! env | grep -q '^GOPATH='; then
	export GOPATH="$HOME/gopath/"
	mkdir "$GOPATH"
	if ! grep -q '^export GOPATH=' ~/.bashrc; then
	        echo "export GOPATH=~/gopath/" >> ~/.bashrc
	fi
	echo "setting go path to: $GOPATH"
fi

echo "gopath is: $GOPATH"

# some versions of golang apparently require this to run go get :(
if ! env | grep -q '^GOBIN='; then
	export GOBIN="${GOPATH}bin/"
	mkdir "$GOBIN"
	if ! grep -q '^export GOBIN=' ~/.bashrc; then
	        echo 'export GOBIN="${GOPATH}bin/"' >> ~/.bashrc
	fi
	echo "setting go bin to: $GOBIN"
fi

echo "gobin is: $GOBIN"

# add gobin to $PATH
if ! env | grep '^PATH=' | grep -q "$GOBIN"; then
	if ! grep -q '^export PATH="'"${GOBIN}:${PATH}"'"' ~/.bashrc; then
	        echo 'export PATH="'"${GOBIN}"':'"${PATH}"'"' >> ~/.bashrc
	fi
	export PATH="${PATH}"	# basically useless
	echo "setting path to: $PATH"
fi

echo "path is: $PATH"

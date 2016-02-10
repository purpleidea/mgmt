#!/bin/bash
# setup a few environment path values

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
	if ! grep -q '^export PATH="'"${GOBIN}"':${PATH}"' ~/.bashrc; then
	        echo 'export PATH="'"${GOBIN}"':${PATH}"' >> ~/.bashrc
	fi
	export PATH="${GOBIN}:${PATH}"
	echo "setting path to: $PATH"
fi

echo "path is: $PATH"

# add ~/bin/ to $PATH
if ! env | grep '^PATH=' | grep -q "$HOME/bin"; then
	mkdir -p "${HOME}/bin"
	if ! grep -q '^export PATH="'"${HOME}/bin"':${PATH}"' ~/.bashrc; then
	        echo 'export PATH="'"${HOME}/bin"':${PATH}"' >> ~/.bashrc
	fi
	export PATH="${HOME}/bin:${PATH}"
	echo "setting path to: $PATH"
fi

echo "path is: $PATH"

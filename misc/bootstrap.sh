#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# Configure your local dev environment for this git repo.
# Invoke as "misc/bootstrap".
################################################################################

# Update git submodules.
smitty git submodule sync
smitty git submodule update --init --recursive

# Clean/reset git submodules.
warn "About to clean git submodules."
echo "Press ^C within 3s to abort."
sleep 3s
smitty git clean -ffd

# Install a specific version of gometalinter.
smitty go get -u gopkg.in/alecthomas/gometalinter.v1
dir="$(dirname $(which gometalinter.v1))"
mv "${dir}/gometalinter.v1" "${dir}/gometalinter"

# Install or update go dependencies.
smitty go get golang.org/x/tools/cmd/goimports
smitty go get github.com/golang/lint/golint
smitty go get golang.org/x/tools/cmd/gotype
smitty go get github.com/client9/misspell/cmd/misspell
smitty go get github.com/mdempsky/unconvert
smitty go get golang.org/x/tools/cmd/stringer
smitty go get github.com/jteeuwen/go-bindata/go-bindata

# Create or update go source in bindata/
make bindata

# Fetch dependent go source.
smitty go get -d ./...

# https://git-scm.com/docs/git-gc
smitty git gc --prune
smitty git fetch --all --prune

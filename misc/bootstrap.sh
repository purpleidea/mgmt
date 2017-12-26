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

# Add an "upstream" git remote.
UPSTREAM_URI="https://github.com/purpleidea/mgmt.git"
if git remote show upstream 2> /dev/null | grep "${UPSTREAM_URI}" &> /dev/null; then
	info upstream git remote is OK
else
	git remote rm upstream &> /dev/null || :
	warn "Setting upstream remote."
	smitty git remote add upstream "${UPSTREAM_URI}"
fi

# Allow to easily checkout Pull Requests (PRs) locally.
if [[ "$(git config --list)" =~ remote.upstream.fetch=\+refs/pull/\*/head:refs/remotes/upstream/pr/\* ]]; then
	info config to fetch upstream PR is OK
else
	warn "Configuring git to fetch pull requests. See \"misc/checkout-pr.sh\" for info."
	smitty git config --add remote.upstream.fetch '+refs/pull/*/head:refs/remotes/upstream/pr/*'
fi

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

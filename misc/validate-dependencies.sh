#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# Fail if the test environment lacks any dependencies.
################################################################################

# If golang is too old, we don't want to fail with an obscure error later.
if go version | grep 'go1\.[012345]\.'; then
	echo "mgmt requires go1.6 or higher."
	exit 1
fi

declare -r commands="
	awk
	file
	go
	goimports
	golint
	gometalinter
	gotype
	misspell
	sed
	unconvert
	which
"
declare missing_commands=""

for command in ${commands}; do
	if ! command -v "${command}" &> /dev/null; then
		missing_commands="${missing_commands} ${command}"
	fi
done

if [[ -n "${missing_commands}" ]]; then
	err "Missing commands:${missing_commands}"
	info "Try running \"make deps\"."
	exit 1
fi

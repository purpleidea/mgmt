#!/bin/bash
set -eEu
set -o pipefail
. test/util.sh

################################################################################
# Checkout a pull request locally.
# Invoke as "misc/checkout-pr.sh <pr_number>".
# Example:
#   misc/checkout-pr.sh 99
################################################################################

if [[ $# -ne 1 ]]; then
	err "You must provide a single argument, the upstream pull request number."
	indent "Example:"
	indent "  misc/checkout-pr.sh 99"
	exit 1
fi

declare -ri pr_number=$1
smitty git checkout -t "upstream/pr/${pr_number}"
smitty git reset --hard "upstream/pr/${pr_number}"
if [[ -x misc/bootstrap.sh ]]; then
	misc/bootstrap.sh
else
	smitty git submodule sync
	smitty git submodule update --init --recursive
	smitty git clean -ffdx
fi

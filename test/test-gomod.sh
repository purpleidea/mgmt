#!/usr/bin/env bash
# Mgmt
# Copyright (C) James Shubin and the project contributors
# Written by James Shubin <james@shubin.ca> and the project contributors
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.
#
# Additional permission under GNU GPL version 3 section 7
#
# If you modify this program, or any covered work, by linking or combining it
# with embedded mcl code and modules (and that the embedded mcl code and
# modules which link with this program, contain a copy of their source code in
# the authoritative form) containing parts covered by the terms of any other
# license, the licensors of this program grant you additional permission to
# convey the resulting work. Furthermore, the licensors of this program grant
# the original author, James Shubin, additional permission to update this
# additional permission if he deems it necessary to achieve the goals of this
# additional permission.

echo running "$0"
set -o errexit
#set -o nounset
set -o pipefail

ROOT=$(dirname "${BASH_SOURCE}")/..
cd "${ROOT}"
. test/util.sh

# 1. Run the upgrade script
if ! ./misc/gomodupgrade.sh; then
	fail_test "Failed to run ./misc/gomodupgrade.sh"
fi

# 2. Check for changes
# We use --exit-code to return 1 if there are changes
if ! git diff --exit-code go.mod go.sum > /dev/null; then
	git diff go.mod go.sum
	fail_test "go.mod or go.sum are not up to date. Run ./misc/gomodupgrade.sh to update them."
fi

# 3. Check if it still builds
# We use 'go build ./...' to ensure everything compiles
if ! go build ./...; then
	fail_test "Build failed after dependency upgrade."
fi

echo 'PASS'

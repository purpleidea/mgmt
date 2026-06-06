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

set -e

# Instead of relying on a single bulk upgrade which might fail due to "weird
# corner cases" (e.g. incompatible Go versions, retracted sub-packages, or
# other upstream issues in a single transitive dependency), we attempt a bulk
# upgrade first. If that fails, we fallback to upgrading each direct
# dependency individually so that valid upgrades aren't blocked by one failure.

if ! go get -u -t ./...; then
	echo "Bulk upgrade failed, attempting module-by-module upgrade..."
	# Get all direct dependencies of the project
	# We use awk since `go list` output formatting can sometimes be tricky
	# with large dependency graphs.
	direct_mods=$(go list -m -f '{{if not (or .Main .Indirect)}}{{.Path}}{{end}}' all)

	for mod in $direct_mods; do
		echo "Upgrading $mod..."
		# -u: use the network to find the latest versions
		# -t: also consider modules needed to build tests
		go get -u -t "$mod" || echo "Warning: failed to upgrade $mod, skipping."
	done
fi

# tidy up and remove unused dependencies
go mod tidy

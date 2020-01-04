# Mgmt
# Copyright (C) 2013-2020+ James Shubin and the project contributors
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
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# The bindata target generates go files from any source defined below. To use
# the files, import the generated "bindata" package and use:
# `bytes, err := bindata.Asset("FILEPATH")`
# where FILEPATH is the path of the original input file relative to `bindata/`.
# To get a list of files stored in this "bindata" package, you can use:
# `paths := bindata.AssetNames()` and `paths, err := bindata.AssetDir(name)`
# to get a list of files with a directory prefix.

.PHONY: build clean
default: build

build: bindata.go

# add more input files as dependencies at the end here...
bindata.go: ../COPYING
	@echo "Generating: bindata..."
	# go-bindata --pkg bindata -o <OUTPUT> <INPUT>
	go-bindata --pkg bindata -o ./$@ $^
	# gofmt the output file
	gofmt -s -w $@
	@ROOT=$$(dirname "$${BASH_SOURCE}")/.. && $$ROOT/misc/header.sh '$@'

clean:
	# remove generated bindata.go
	@ROOT=$$(dirname "$${BASH_SOURCE}")/.. && rm -f bindata.go

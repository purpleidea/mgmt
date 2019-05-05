# Mgmt
# Copyright (C) 2013-2019+ James Shubin and the project contributors
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

SHELL = /usr/bin/env bash
.PHONY: all build clean

all: build

build: kind_stringer.go

clean:
	@rm -f kind_stringer.go || true

kind_stringer.go: type.go
	@echo "Generating: type kind strings..."
	@# workaround `stringer` regression in golang 1.11
	@# see: https://github.com/golang/go/issues/31843
	@unset GOCACHE && go generate # GOCACHE is returned to normal on exit...

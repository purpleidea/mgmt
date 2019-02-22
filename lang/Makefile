# Mgmt
# Copyright (C) 2013-2018+ James Shubin and the project contributors
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

OLDGOYACC := $(shell go version | grep -E 'go1.6|go1.7')

all: build

build: lexer.nn.go y.go

clean:
	@rm -f lexer.nn.go y.go y.output || true

lexer.nn.go: lexer.nex
	@echo "Generating: lexer..."
	nex -e lexer.nex
	@ROOT="$$( cd "$$( dirname "$${BASH_SOURCE[0]}" )" && cd .. && pwd )" && $$ROOT/misc/header.sh 'lexer.nn.go'

y.go: parser.y
	@echo "Generating: parser..."
ifneq ($(OLDGOYACC),)
	go tool yacc parser.y
else
	@# newer golang versions moved yacc into: golang.org/x/tools/cmd/goyacc
	goyacc parser.y
endif
	@ROOT="$$( cd "$$( dirname "$${BASH_SOURCE[0]}" )" && cd .. && pwd )" && $$ROOT/misc/header.sh 'y.go'

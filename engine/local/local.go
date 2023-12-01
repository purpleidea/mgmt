// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package local contains functions and interfaces that are shared between
// functions and resources. It's similar to the "world" functionality, except
// that it only involves local operations that stay within a single machine or
// local mgmt instance.
package local

import (
	"context"
)

// API implements the base handle for all the methods in this package. If we
// were going to have more than one implementation for all of these, then this
// would be an interface instead, and different packages would implement it.
// Since this is not the expectation for the local API, it's all self-contained.
type API struct {
	Prefix string
	Debug  bool
	Logf   func(format string, v ...interface{})
}

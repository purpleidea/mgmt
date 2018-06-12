// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// Package funcs provides a framework for functions that change over time.
package funcs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

// registeredFuncs is a global map of all possible funcs which can be used. You
// should never touch this map directly. Use methods like Register instead. It
// includes implementations which also satisfy PolyFunc as well.
var registeredFuncs = make(map[string]func() interfaces.Func) // must initialize

// Register takes a func and its name and makes it available for use. It is
// commonly called in the init() method of the func at program startup. There is
// no matching Unregister function. You may also register functions which
// satisfy the PolyFunc interface.
func Register(name string, fn func() interfaces.Func) {
	if _, exists := registeredFuncs[name]; exists {
		panic(fmt.Sprintf("a func named %s is already registered", name))
	}
	//gob.Register(fn())
	registeredFuncs[name] = fn
}

// Lookup returns a pointer to the function's struct. It may be convertible to a
// PolyFunc if the particular function implements those additional methods.
func Lookup(name string) (interfaces.Func, error) {
	f, exists := registeredFuncs[name]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return f(), nil
}

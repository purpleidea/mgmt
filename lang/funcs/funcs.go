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
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

const (
	// ModuleSep is the character used for the module scope separation. For
	// example when using `fmt.printf` or `math.sin` this is the char used.
	// It is included here for convenience when importing this package.
	ModuleSep = interfaces.ModuleSep
)

// registeredFuncs is a global map of all possible funcs which can be used. You
// should never touch this map directly. Use methods like Register instead. It
// includes implementations which also satisfy PolyFunc as well.
var registeredFuncs = make(map[string]func() interfaces.Func) // must initialize

// Register takes a func and its name and makes it available for use. It is
// commonly called in the init() method of the func at program startup. There is
// no matching Unregister function. You may also register functions which
// satisfy the PolyFunc interface. To register a function which lives in a
// module, you must join the module name to the function name with the ModuleSep
// character. It is defined as a const and is probably the period character.
func Register(name string, fn func() interfaces.Func) {
	if _, exists := registeredFuncs[name]; exists {
		panic(fmt.Sprintf("a func named %s is already registered", name))
	}

	// can't contain more than one period in a row
	if strings.Index(name, ModuleSep+ModuleSep) >= 0 {
		panic(fmt.Sprintf("a func named %s is invalid", name))
	}
	// can't start or end with a period
	if strings.HasPrefix(name, ModuleSep) || strings.HasSuffix(name, ModuleSep) {
		panic(fmt.Sprintf("a func named %s is invalid", name))
	}

	//gob.Register(fn())
	registeredFuncs[name] = fn
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module.
func ModuleRegister(module, name string, fn func() interfaces.Func) {
	Register(module+ModuleSep+name, fn)
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

// LookupPrefix returns a map of names to functions that start with a module
// prefix. This search automatically adds the period separator. So if you want
// functions in the `fmt` package, search for `fmt`, not `fmt.` and it will find
// all the correctly registered functions. This removes that prefix from the
// result in the map keys that it returns. If you search for an empty prefix,
// then this will return all the top-level functions that aren't in a module.
func LookupPrefix(prefix string) map[string]func() interfaces.Func {
	result := make(map[string]func() interfaces.Func)
	for name, f := range registeredFuncs {
		// requested top-level functions, and no module separators...
		if prefix == "" {
			if !strings.Contains(name, ModuleSep) {
				result[name] = f // copy
			}
			continue
		}
		sep := prefix + ModuleSep
		if !strings.HasPrefix(name, sep) {
			continue
		}
		s := strings.TrimPrefix(name, sep) // remove the prefix
		result[s] = f                      // copy
	}
	return result
}

// Map returns a map from all registered function names to a function to return
// that one. We return a copy of our internal registered function store so that
// this result can be manipulated safely. We return the functions that produce
// the Func interface because we might use this result to create multiple
// functions, and each one must have its own unique memory address to work
// properly.
func Map() map[string]func() interfaces.Func {
	m := make(map[string]func() interfaces.Func)
	for name, fn := range registeredFuncs { // copy
		m[name] = fn
	}
	return m
}

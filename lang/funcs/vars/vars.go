// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

// Package vars provides a framework for language vars.
package vars

import (
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/interfaces"
)

const (
	// ConstNamespace is the string prefix for all top-level built-in vars.
	ConstNamespace = "const"

	// ResourceNamespace is the string prefix for all top-level resource
	// specific built-in vars, that exist under the ConstNamespace header.
	ResourceNamespace = "res"
)

// registeredVars is a global map of all possible vars which can be used. You
// should never touch this map directly. Use methods like Register instead.
var registeredVars = make(map[string]func() interfaces.Var) // must initialize

// Register takes a var and its name and makes it available for use. It is
// commonly called in the init() method of the var at program startup. There is
// no matching Unregister function.
func Register(name string, fn func() interfaces.Var) {
	if _, ok := registeredVars[name]; ok {
		panic(fmt.Sprintf("a var named %s is already registered", name))
	}
	//gob.Register(fn())
	registeredVars[name] = fn
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
//func ModuleRegister(module, name string, v func() interfaces.Var) {
//	Register(module+interfaces.ModuleSep+name, v)
//}

// resourceConstHelper is a helper function to manage the const topology.
func resourceConstHelper(kind, param, field string) string {
	// const.res.file.state.exists = "exists"
	// TODO: should it be: const.res.file.params.state.exists = "exists" ?
	chunks := []string{
		ConstNamespace,
		ResourceNamespace,
		kind,
		param,
		field,
	}
	//return ConstNamespace + interfaces.ModuleSep + ResourceNamespace + interfaces.ModuleSep + kind + interfaces.ModuleSep + param + interfaces.ModuleSep + field
	return strings.Join(chunks, interfaces.ModuleSep) // cleaner code
}

// RegisterResourceParam registers a single const param for a resource. You
// might prefer to use RegisterResourceParams instead.
func RegisterResourceParam(kind, param, field string, value func() interfaces.Var) {
	Register(resourceConstHelper(kind, param, field), value)
}

// RegisterResourceParams registers a map of const params for a resource. The
// params mapping keys are the param name and the param field name. Finally, the
// value is the specific type value for that constant.
func RegisterResourceParams(kind string, params map[string]map[string]func() interfaces.Var) {
	for param, mapping := range params {
		for field, value := range mapping {
			Register(resourceConstHelper(kind, param, field), value)
		}
	}
}

// Lookup returns a pointer to the var implementation.
func Lookup(name string) (interfaces.Var, error) {
	f, exists := registeredVars[name]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return f(), nil
}

// LookupPrefix returns a map of names to vars that start with a module prefix.
// This search automatically adds the period separator. So if you want vars in
// the `const` prefix, search for `const`, not `const.` and it will find all the
// correctly registered vars. This removes that prefix from the result in the
// map keys that it returns. If you search for an empty prefix, then this will
// return all the top-level functions that aren't in a module.
func LookupPrefix(prefix string) map[string]func() interfaces.Var {
	result := make(map[string]func() interfaces.Var)
	for name, f := range registeredVars {
		// requested top-level vars, and no module separators...
		if prefix == "" {
			if !strings.Contains(name, interfaces.ModuleSep) {
				result[name] = f // copy
			}
			continue
		}
		sep := prefix + interfaces.ModuleSep
		if !strings.HasPrefix(name, sep) {
			continue
		}
		s := strings.TrimPrefix(name, sep) // remove the prefix
		result[s] = f                      // copy
	}
	return result
}

// Map returns a map from all registered var names to a function to return that
// one. We return a copy of our internal registered var store so that this
// result can be manipulated safely. We return the vars that produce the Var
// interface because we might use this result to create multiple vars, and each
// one might need to have its own unique memory address to work properly.
func Map() map[string]func() interfaces.Var {
	m := make(map[string]func() interfaces.Var)
	for name, fn := range registeredVars { // copy
		m[name] = fn
	}
	return m
}

// Mgmt
// Copyright (C) James Shubin and the project contributors
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
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//
// Additional permission under GNU GPL version 3 section 7
//
// If you modify this program, or any covered work, by linking or combining it
// with embedded mcl code and modules (and that the embedded mcl code and
// modules which link with this program, contain a copy of their source code in
// the authoritative form) containing parts covered by the terms of any other
// license, the licensors of this program grant you additional permission to
// convey the resulting work. Furthermore, the licensors of this program grant
// the original author, James Shubin, additional permission to update this
// additional permission if he deems it necessary to achieve the goals of this
// additional permission.

// Package funcs provides a framework for functions that change over time.
package funcs

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"
)

const (
	// ModuleSep is the character used for the module scope separation. For
	// example when using `fmt.printf` or `math.sin` this is the char used.
	// It is included here for convenience when importing this package.
	ModuleSep = interfaces.ModuleSep

	// ReplaceChar is a special char that is used to replace ModuleSep when
	// it can't be used for some reason. This currently only happens in the
	// golang template library. Even with this limitation in that library,
	// we don't want to allow this as the first or last character in a name.
	// NOTE: the template library will panic if it is one of: .-#
	ReplaceChar = "_"

	// CoreDir is the directory prefix where core mcl code is embedded.
	CoreDir = "core/"

	// FunctionsRelDir is the path where the functions are kept, relative to
	// the main source code root.
	FunctionsRelDir = "lang/core/"

	// ConcatFuncName is the name the concat function is registered as. It
	// is listed here because it needs a well-known name that can be used by
	// the string interpolation code.
	ConcatFuncName = "concat"

	// ContainsFuncName is the name the contains function is registered as.
	ContainsFuncName = "contains"

	// LookupDefaultFuncName is the name this function is registered as.
	// This starts with an underscore so that it cannot be used from the
	// lexer.
	LookupDefaultFuncName = "_lookup_default"

	// LookupFuncName is the name this function is registered as.
	// This starts with an underscore so that it cannot be used from the
	// lexer.
	LookupFuncName = "_lookup"

	// StructLookupFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	StructLookupFuncName = "_struct_lookup"

	// StructLookupOptionalFuncName is the name this function is registered
	// as. This starts with an underscore so that it cannot be used from the
	// lexer.
	StructLookupOptionalFuncName = "_struct_lookup_optional"

	// CollectFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	CollectFuncName = "_collect"

	// CollectFuncInFieldName is the name of the name field in the struct.
	CollectFuncInFieldName = "name"
	// CollectFuncInFieldHost is the name of the host field in the struct.
	CollectFuncInFieldHost = "host"

	// CollectFuncInType is the most complex of the three possible input
	// types. The other two possible ones are str or []str.
	CollectFuncInType = "[]struct{" + CollectFuncInFieldName + " str; " + CollectFuncInFieldHost + " str}"

	// CollectFuncOutFieldName is the name of the name field in the struct.
	CollectFuncOutFieldName = "name"
	// CollectFuncOutFieldHost is the name of the host field in the struct.
	CollectFuncOutFieldHost = "host"
	// CollectFuncOutFieldData is the name of the data field in the struct.
	CollectFuncOutFieldData = "data"

	// CollectFuncOutStruct is the struct type that we return a list of.
	CollectFuncOutStruct = "struct{" + CollectFuncOutFieldName + " str; " + CollectFuncOutFieldHost + " str; " + CollectFuncOutFieldData + " str}"

	// CollectFuncOutType is the expected return type, the data field is an
	// encoded resource blob.
	CollectFuncOutType = "[]" + CollectFuncOutStruct

	// ErrCantSpeculate is an error that explains that we can't speculate
	// when trying to Call a function. This often gets called by the Value()
	// method of the Expr. This can be useful if we want to distinguish
	// between "something is broken" and "I just can't produce a value at
	// this time", which can be identified and skipped over. If it's the
	// former, then it's okay to error early and shut everything down since
	// we know this function is never going to work the way it's called.
	ErrCantSpeculate = util.Error("can't speculate")
)

// registeredFuncs is a global map of all possible funcs which can be used. You
// should never touch this map directly. Use methods like Register instead. It
// includes implementations which also satisfy BuildableFunc and InferableFunc
// as well.
var registeredFuncs = make(map[string]func() interfaces.Func) // must initialize

// Register takes a func and its name and makes it available for use. It is
// commonly called in the init() method of the func at program startup. There is
// no matching Unregister function. You may also register functions which
// satisfy the BuildableFunc and InferableFunc interfaces. To register a
// function which lives in a module, you must join the module name to the
// function name with the ModuleSep character. It is defined as a const and is
// probably the period character.
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
	// TODO: this should be added but conflicts with our internal functions
	// can't start or end with an underscore
	//if strings.HasPrefix(name, ReplaceChar) || strings.HasSuffix(name, ReplaceChar) {
	//	panic(fmt.Sprintf("a func named %s is invalid", name))
	//}

	//gob.Register(fn())
	registeredFuncs[name] = fn

	f := fn() // Remember: If we modify this copy, it gets thrown away!

	if _, ok := f.(interfaces.MetadataFunc); ok { // If it does it itself...
		return
	}

	// We have to do it manually...
	metadata, err := GetFunctionMetadata(f)
	if err != nil {
		panic(fmt.Sprintf("could not get function metadata for %s: %v", name, err))
	}

	if err := docsUtil.RegisterFunction(name, metadata); err != nil {
		panic(fmt.Sprintf("could not register function metadata for %s", name))
	}
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module.
func ModuleRegister(module, name string, fn func() interfaces.Func) {
	Register(module+ModuleSep+name, fn)
}

// Lookup returns a pointer to the function's struct. It may be convertible to a
// BuildableFunc or InferableFunc if the particular function implements those
// additional methods.
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

// GetFunctionName reads the handle to find the underlying real function name.
// The function can be an actual function or a struct which implements one.
func GetFunctionName(fn interface{}) string {
	pc := runtime.FuncForPC(reflect.ValueOf(fn).Pointer())
	if pc == nil {
		// This part works for structs, the other parts work for funcs.
		t := reflect.TypeOf(fn)
		if t.Kind() == reflect.Ptr {
			t = t.Elem()
		}

		return t.Name()
	}

	// if pc.Name() is: github.com/purpleidea/mgmt/lang/core/math.Pow
	sp := strings.Split(pc.Name(), "/")

	// ...this will be: math.Pow
	s := sp[len(sp)-1]

	ix := strings.LastIndex(s, ".")
	if ix == -1 { // standalone
		return s
	}

	// ... this will be: Pow
	return s[ix+1:]
}

// GetFunctionMetadata builds a metadata struct with everything about this func.
func GetFunctionMetadata(fn interface{}) (*docsUtil.Metadata, error) {
	nested := 1 // because this is wrapped in a function
	// Additional metadata for documentation generation!
	_, self, _, ok := runtime.Caller(0 + nested)
	if !ok {
		return nil, fmt.Errorf("could not locate function filename (1)")
	}
	depth := 1 + nested
	// If this is ModuleRegister, we look deeper! Normal Register is depth 1
	filename := self // initial condition to start the loop
	for filename == self {
		_, filename, _, ok = runtime.Caller(depth)
		if !ok {
			return nil, fmt.Errorf("could not locate function filename (2)")
		}
		depth++
	}

	// Get the function implementation path relative to FunctionsRelDir.
	// FIXME: Technically we should split this by dirs instead of using
	// string indexing, which is less correct, but we control the dirs.
	ix := strings.LastIndex(filename, FunctionsRelDir)
	if ix == -1 {
		return nil, fmt.Errorf("could not locate function filename (3): %s", filename)
	}
	filename = filename[ix+len(FunctionsRelDir):]

	funcname := GetFunctionName(fn)

	return &docsUtil.Metadata{
		Filename: filename,
		Typename: funcname,
	}, nil
}

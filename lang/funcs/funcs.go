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
	"sync"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
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
	// TODO: this should be added but conflicts with our internal functions
	// can't start or end with an underscore
	//if strings.HasPrefix(name, ReplaceChar) || strings.HasSuffix(name, ReplaceChar) {
	//	panic(fmt.Sprintf("a func named %s is invalid", name))
	//}

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

// PureFuncExec is usually used to provisionally speculate about the result of a
// pure function, by running it once, and returning the result. Pure functions
// are expected to only produce one value that depends only on the input values.
// This won't run any slow functions either.
func PureFuncExec(handle interfaces.Func, args []types.Value) (types.Value, error) {
	hostname := ""                                   // XXX: add to interface
	debug := false                                   // XXX: add to interface
	logf := func(format string, v ...interface{}) {} // XXX: add to interface

	info := handle.Info()
	if !info.Pure {
		return nil, fmt.Errorf("func is not pure")
	}
	// if function is expensive to run, we won't run it provisionally
	if info.Slow {
		return nil, fmt.Errorf("func is slow")
	}

	if err := handle.Validate(); err != nil {
		return nil, errwrap.Wrapf(err, "could not validate func")
	}

	sig := handle.Info().Sig
	if sig.Kind != types.KindFunc {
		return nil, fmt.Errorf("must be kind func")
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	errch := make(chan error)
	input := make(chan types.Value)  // we close this when we're done
	output := make(chan types.Value) // we create it, func closes it

	init := &interfaces.Init{
		Hostname: hostname,
		Input:    input,
		Output:   output,
		World:    nil, // should not be used for pure functions
		Debug:    debug,
		Logf: func(format string, v ...interface{}) {
			logf("func: "+format, v...)
		},
	}

	if err := handle.Init(init); err != nil {
		return nil, errwrap.Wrapf(err, "could not init func")
	}

	close1 := make(chan struct{})
	close2 := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(errch) // last one turns out the lights
		select {
		case <-close1:
		}
		select {
		case <-close2:
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(close1)
		if debug {
			logf("Running func")
		}
		err := handle.Stream() // sends to output chan
		if debug {
			logf("Exiting func")
		}
		if err == nil {
			return
		}
		// we closed with an error...
		select {
		case errch <- errwrap.Wrapf(err, "problem streaming func"):
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(close2)
		defer close(input) // we only send one value
		if len(args) == 0 {
			return
		}
		si := &types.Type{
			// input to functions are structs
			Kind: types.KindStruct,
			Map:  handle.Info().Sig.Map,
			Ord:  handle.Info().Sig.Ord,
		}
		st := types.NewStruct(si)

		for i, arg := range args {
			name := util.NumToAlpha(i)                // assume (incorrectly) for now...
			if err := st.Set(name, arg); err != nil { // populate struct
				select {
				case errch <- errwrap.Wrapf(err, "struct set failure"):
				}
				return
			}
		}

		select {
		case input <- st: // send to function (must not block)
		case <-close1: // unblock the input send in case stream closed
			select {
			case errch <- fmt.Errorf("stream closed early"):
			}
		}
	}()

	once := false
	var result types.Value
	var reterr error
Loop:
	for {
		select {
		case value, ok := <-output: // read from channel
			if !ok {
				output = nil
				continue Loop // only exit via errch closing!
			}
			if once {
				reterr = fmt.Errorf("got more than one value")
				continue // only exit via errch closing!
			}
			once = true
			result = value // save value

		case err, ok := <-errch: // handle possible errors
			if !ok {
				break Loop
			}
			e := errwrap.Wrapf(err, "problem streaming func")
			reterr = errwrap.Append(reterr, e)
		}
	}

	if err := handle.Close(); err != nil {
		err = errwrap.Append(err, reterr)
		return nil, errwrap.Wrapf(err, "problem closing func")
	}

	return result, reterr
}

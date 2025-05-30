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

// Package facts provides a framework for language values that change over time.
package facts

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ErrCantSpeculate is an error that explains that we can't speculate
	// when trying to Call a function. This often gets called by the Value()
	// method of the Expr. This can be useful if we want to distinguish
	// between "something is broken" and "I just can't produce a value at
	// this time", which can be identified and skipped over. If it's the
	// former, then it's okay to error early and shut everything down since
	// we know this function is never going to work the way it's called.
	ErrCantSpeculate = funcs.ErrCantSpeculate
)

// registeredFacts is a global map of all possible facts which can be used. You
// should never touch this map directly. Use methods like Register instead.
var registeredFacts = make(map[string]struct{}) // must initialize

// Register takes a fact and its name and makes it available for use. It is
// commonly called in the init() method of the fact at program startup. There is
// no matching Unregister function.
func Register(name string, fn func() Fact) {
	if _, ok := registeredFacts[name]; ok {
		panic(fmt.Sprintf("a fact named %s is already registered", name))
	}
	f := fn() // don't wrap this more than once!

	metadata, err := funcs.GetFunctionMetadata(f)
	if err != nil {
		panic(fmt.Sprintf("could not locate fact filename for %s", name))
	}

	//gob.Register(fn())
	funcs.Register(name, func() interfaces.Func { // implement in terms of func interface
		return &FactFunc{
			Fact: fn(), // this MUST be a fresh/unique pointer!

			Metadata: metadata,
		}
	})
	registeredFacts[name] = struct{}{}
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
func ModuleRegister(module, name string, fn func() Fact) {
	Register(module+funcs.ModuleSep+name, fn)
}

// Info is a static representation of some information about the fact. It is
// used for static analysis and type checking. If you break this contract, you
// might cause a panic.
type Info struct {
	Pure   bool        // is the function pure? (can it be memoized?)
	Memo   bool        // should the function be memoized? (false if too much output)
	Fast   bool        // is the function fast? (avoid speculative execution)
	Spec   bool        // can we speculatively execute it? (true for most)
	Output *types.Type // output value type (must not change over time!)
	Err    error       // did this fact validate?
}

// Init is the structure of values and references which is passed into all facts
// on initialization.
type Init struct {
	Hostname string // uuid for the host
	//Noop bool
	Output chan types.Value // Stream must close `output` chan
	World  engine.World
	Debug  bool
	Logf   func(format string, v ...interface{})
}

// Fact is the interface that any valid fact must fulfill. It is very simple,
// but still event driven. Facts should attempt to only send values when they
// have changed.
// TODO: should we support a static version of this interface for facts that
// never change to avoid the overhead of the goroutine and channel listener?
// TODO: should we move this to the interface package?
type Fact interface {
	String() string
	//Validate() error // currently not needed since no facts are internal
	Info() *Info
	Init(*Init) error
	Stream(context.Context) error
}

// CallableFact is a function that takes no args, and that can be called
// statically if we want to do it speculatively or from a resource.
type CallableFact interface {
	Fact // implement everything in Fact but add the additional requirements

	// Call this fact and return the value if it is possible to do so at
	// this time.
	Call(ctx context.Context) (types.Value, error)
}

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

// Package facts provides a framework for language values that change over time.
package facts

import (
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// RegisteredFacts is a global map of all possible facts which can be used. You
// should never touch this map directly. Use methods like Register instead.
var RegisteredFacts = make(map[string]func() Fact) // must initialize

// Register takes a fact and its name and makes it available for use. It is
// commonly called in the init() method of the fact at program startup. There is
// no matching Unregister function.
func Register(name string, fn func() Fact) {
	if _, ok := RegisteredFacts[name]; ok {
		panic(fmt.Sprintf("a fact named %s is already registered", name))
	}
	//gob.Register(fn())
	funcs.Register(name, func() interfaces.Func { // implement in terms of func interface
		return &FactFunc{
			Fact: fn(),
		}
	})
	RegisteredFacts[name] = fn
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
	Output *types.Type // output value type (must not change over time!)
	Err    error       // did this fact validate?
}

// Init is the structure of values and references which is passed into all
// facts on initialization.
type Init struct {
	Hostname string // uuid for the host
	//Noop bool
	Output chan types.Value // Stream must close `output` chan
	World  engine.World
}

// Fact is the interface that any valid fact must fulfill. It is very simple,
// but still event driven. Facts should attempt to only send values when they
// have changed.
// TODO: should we support a static version of this interface for facts that
// never change to avoid the overhead of the goroutine and channel listener?
// TODO: should we move this to the interface package?
type Fact interface {
	//Validate() error // currently not needed since no facts are internal
	Info() *Info
	Init(*Init) error
	Stream() error
	Close() error
}

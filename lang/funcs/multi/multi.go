// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package multi

import (
	"fmt"
	"sort"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	unificationUtil "github.com/purpleidea/mgmt/lang/unification/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// RegisteredFuncs maps a function name to the corresponding function scaffold.
var RegisteredFuncs = make(map[string]*Scaffold) // must initialize

// Scaffold holds the necessary data to build a (possibly polymorphic) function
// with this API.
type Scaffold struct {
	// T is the type of the function. It can include unification variables.
	// At a minimum, this must be a `func(?1) ?2` as a naked `?1` is not
	// allowed. (TODO: Because of ArgGen.)
	T *types.Type

	// M is a build function to run after type unification. It will get
	// passed the solved type of this function. It should error if this is
	// not an acceptable option. On success, it should return the
	// implementing function to use. Of note, this API does not tell the
	// implementation what the correct return type should be. If it can't be
	// determined from the input types, then a different function API needs
	// to be used. XXX: Should we extend this here?
	M func(typ *types.Type) (interfaces.FuncSig, error)

	// D is the documentation handle for this function. We look on that
	// struct or function for the doc string.
	D interface{}
}

// Register registers a simple, static, pure, polymorphic function. It is easier
// to use than the raw function API. It allows you to build and check a function
// based on a type signature that contains unification variables. You may only
// specify a single type signature with the API, so some complex patterns are
// not possible with this API. Implementing a function like `printf` would not
// be possible. Implementing a function which counts the number of elements in a
// list would be.
func Register(name string, scaffold *Scaffold) {
	if _, exists := RegisteredFuncs[name]; exists {
		panic(fmt.Sprintf("a simple polyfunc named %s is already registered", name))
	}

	if scaffold == nil {
		panic("no scaffold specified for simple polyfunc")
	}
	if scaffold.T == nil {
		panic("no type specified for simple polyfunc")
	}
	if scaffold.T.Kind != types.KindFunc {
		panic("type must be a func")
	}
	if scaffold.T.HasVariant() {
		panic("func contains a variant type signature")
	}
	if scaffold.M == nil {
		panic("no implementation specified for simple polyfunc")
	}

	RegisteredFuncs[name] = scaffold // store a copy for ourselves

	metadata, err := funcs.GetFunctionMetadata(scaffold.D)
	if err != nil {
		panic(fmt.Sprintf("could not locate function filename for %s", name))
	}

	// register a copy in the main function database
	funcs.Register(name, func() interfaces.Func {
		return &Func{
			Metadata: metadata,
			WrappedFunc: &wrapped.Func{
				Name: name,
				// NOTE: It might be more correct to Copy here,
				// but we do the copy inside of ExprFunc.Copy()
				// instead, so that the same type can be unified
				// in more than one way. Doing it here wouldn't
				// be harmful, but it's an extra copy we don't
				// need to do AFAICT.
				Type: scaffold.T, // .Copy(),
			},
			Make: scaffold.M,
		}
	})
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
func ModuleRegister(module, name string, scaffold *Scaffold) {
	Register(module+funcs.ModuleSep+name, scaffold)
}

// WrappedFunc is a type alias so that we can embed `wrapped.Func` inside our
// struct, since the Func name collides with our Func field name.
type WrappedFunc = wrapped.Func

var _ interfaces.BuildableFunc = &Func{} // ensure it meets this expectation

// Func is a scaffolding function struct which fulfills the boiler-plate for the
// function API, but that can run a very simple, static, pure, polymorphic
// function. This function API is unique in that it lets you provide your own
// `Make` builder function to create the function implementation.
type Func struct {
	*docsUtil.Metadata
	*WrappedFunc // *wrapped.Func as a type alias to pull in the base impl.

	// Make is a build function to run after type unification. It will get
	// passed the solved type of this function. It should error if this is
	// not an acceptable option. On success, it should return the
	// implementing function to use. Of note, this API does not tell the
	// implementation what the correct return type should be. If it can't be
	// determined from the input types, then a different function API needs
	// to be used. XXX: Should we extend this here?
	Make func(typ *types.Type) (interfaces.FuncSig, error)
}

// Build is run to turn the maybe polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after unification
// completes, and must be run before Info() and any of the other Func interface
// methods are used.
func (obj *Func) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...

	f, err := obj.Make(typ)
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't build %s with %s", obj.Name, typ)
	}

	fn := &types.FuncValue{
		T: typ,
		V: f, // implementation
	}
	obj.Fn = fn
	return obj.Fn.T, nil
}

// TypeMatch accepts a map of possible type signatures to corresponding
// implementing functions that we want to check against after type unification.
// On success it returns the function who's corresponding signature matched.
// This helper function returns a function which is suitable for use in the
// scaffold make function field.
func TypeMatch(m map[string]interfaces.FuncSig) func(*types.Type) (interfaces.FuncSig, error) {
	return func(typ *types.Type) (interfaces.FuncSig, error) {
		// sort for determinism in debugging
		keys := []string{}
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, s := range keys {
			t := types.NewType(s)
			if t == nil {
				// TODO: should we panic?
				continue // skip
			}
			if unificationUtil.UnifyCmp(typ, t) == nil {
				return m[s], nil
			}
		}
		return nil, fmt.Errorf("did not match")
	}
}

// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package simplepoly

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// RegisteredFuncs maps a function name to the corresponding static, pure funcs.
var RegisteredFuncs = make(map[string][]*types.FuncValue) // must initialize

// Register registers a simple, static, pure, polymorphic function. It is easier
// to use than the raw function API, but also limits you to small, finite
// numbers of different polymorphic type signatures per function name. You can
// also register functions which return types containing variants, if you want
// automatic matching based on partial types as well. Some complex patterns are
// not possible with this API. Implementing a function like `printf` would not
// be possible. Implementing a function which counts the number of elements in a
// list would be.
func Register(name string, fns []*types.FuncValue) {
	if _, exists := RegisteredFuncs[name]; exists {
		panic(fmt.Sprintf("a simple polyfunc named %s is already registered", name))
	}

	if len(fns) == 0 {
		panic("no functions specified for simple polyfunc")
	}

	// check for uniqueness in type signatures
	typs := []*types.Type{}
	for _, f := range fns {
		if f.T == nil {
			panic(fmt.Sprintf("polyfunc %s contains a nil type signature", name))
		}
		typs = append(typs, f.T)
	}

	if err := hasDuplicateTypes(typs); err != nil {
		panic(fmt.Sprintf("polyfunc %s has a duplicate implementation: %+v", name, err))
	}

	RegisteredFuncs[name] = fns // store a copy for ourselves

	// register a copy in the main function database
	funcs.Register(name, func() interfaces.Func { return &simplePolyFunc{Fns: fns} })
}

// ModuleRegister is exactly like Register, except that it registers within a
// named module. This is a helper function.
func ModuleRegister(module, name string, fns []*types.FuncValue) {
	Register(module+funcs.ModuleSep+name, fns)
}

// simplePolyFunc is a scaffolding function struct which fulfills the
// boiler-plate for the function API, but that can run a very simple, static,
// pure, polymorphic function.
type simplePolyFunc struct {
	Fns []*types.FuncValue // list of possible functions

	fn *types.FuncValue // the concrete version of our chosen function

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output

	closeChan chan struct{}
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *simplePolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	if len(obj.Fns) == 0 {
		return nil, fmt.Errorf("no matching signatures for simple polyfunc")
	}

	// filter out anything that's incompatible with the partialType
	typs := []*types.Type{}
	for _, f := range obj.Fns {
		// TODO: if status is "both", should we skip as too difficult?
		_, err := f.T.ComplexCmp(partialType)
		// XXX: can an f.T with a variant compare with a partial ?
		if err != nil {
			continue
		}
		typs = append(typs, f.T)
	}

	return typs, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used.
func (obj *simplePolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}

	// find typ in obj.Fns
	for ix, f := range obj.Fns {
		if f.T.HasVariant() {
			continue // match these if no direct matches exist
		}
		// FIXME: can we replace this by the complex matcher down below?
		if f.T.Cmp(typ) == nil {
			obj.buildFunction(typ, ix) // found match at this index
			return nil
		}
	}

	// match concrete type against our list that might contain a variant
	var found bool
	for ix, f := range obj.Fns {
		_, err := typ.ComplexCmp(f.T)
		if err != nil {
			continue
		}
		if found { // already found one...
			// TODO: we *could* check that the previous duplicate is
			// equivalent, but in this case, it is really a bug that
			// the function author had by allowing ambiguity in this
			return fmt.Errorf("duplicate match found for build type: %+v", typ)
		}
		found = true
		obj.buildFunction(typ, ix) // found match at this index
	}
	// ensure there's only one match...
	if found {
		return nil // w00t!
	}

	return fmt.Errorf("unable to find a compatible function for type: %+v", typ)
}

// buildFunction builds our concrete static function, from the potentially
// abstract, possibly variant containing list of functions.
func (obj *simplePolyFunc) buildFunction(typ *types.Type, ix int) {
	obj.fn = obj.Fns[ix].Copy().(*types.FuncValue)
	obj.fn.T = typ.Copy() // overwrites any contained "variant" type
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *simplePolyFunc) Validate() error {
	if len(obj.Fns) == 0 {
		return fmt.Errorf("missing list of functions")
	}

	// check for uniqueness in type signatures
	typs := []*types.Type{}
	for _, f := range obj.Fns {
		if f.T == nil {
			return fmt.Errorf("nil type signature found")
		}
		typs = append(typs, f.T)
	}

	if err := hasDuplicateTypes(typs); err != nil {
		return errwrap.Wrapf(err, "duplicate implementation found")
	}

	if obj.fn == nil { // build must be run first
		return fmt.Errorf("a specific function has not been specified")
	}
	if obj.fn.T.Kind != types.KindFunc {
		return fmt.Errorf("func must be a kind of func")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *simplePolyFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: should this be something we specify here?
		Sig:  obj.fn.Type(),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *simplePolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *simplePolyFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				if len(obj.fn.Type().Ord) > 0 {
					return nil // can't output any more
				}
				// no inputs were expected, pass through once
			}
			if ok {
				//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
				//	return errwrap.Wrapf(err, "wrong function input")
				//}

				if obj.last != nil && input.Cmp(obj.last) == nil {
					continue // value didn't change, skip it
				}
				obj.last = input // store for next
			}

			values := []types.Value{}
			for _, name := range obj.fn.Type().Ord {
				x := input.Struct()[name]
				values = append(values, x)
			}

			if obj.init.Debug {
				obj.init.Logf("Calling function with: %+v", values)
			}
			result, err := obj.fn.Call(values) // (Value, error)
			if err != nil {
				if obj.init.Debug {
					obj.init.Logf("Function returned error: %+v", err)
				}
				return errwrap.Wrapf(err, "simple poly function errored")
			}
			if obj.init.Debug {
				obj.init.Logf("Function returned with: %+v", values)
			}

			if obj.result == result {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			if len(obj.fn.Type().Ord) == 0 {
				return nil // no more values, we're a pure func
			}
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *simplePolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}

// hasDuplicateTypes returns an error if the list of types is not unique.
func hasDuplicateTypes(typs []*types.Type) error {
	// FIXME: do this comparison in < O(n^2) ?
	for i, ti := range typs {
		for j, tj := range typs {
			if i == j {
				continue // don't compare to self
			}
			if ti.Cmp(tj) == nil {
				return fmt.Errorf("duplicate type of %+v found", ti)
			}
		}
	}
	return nil
}

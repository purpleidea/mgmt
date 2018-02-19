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

package funcs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

const (
	// ContainsFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	// XXX: change to _contains and add syntax in the lexer/parser
	ContainsFuncName = "contains"
)

func init() {
	Register(ContainsFuncName, func() interfaces.Func { return &ContainsPolyFunc{} }) // must register the func and name
}

// ContainsPolyFunc returns true if a value is found in a list. Otherwise false.
type ContainsPolyFunc struct {
	Type *types.Type // this is the type of value stored in our list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output

	closeChan chan struct{}
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *ContainsPolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as arg for now -- maybe there's a better way?
	variant := []*types.Type{types.NewType("func(needle variant, haystack variant) bool")}

	if partialType == nil {
		return variant, nil
	}

	var typ *types.Type

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 2 {
			return nil, fmt.Errorf("must have exactly three args in contains func")
		}
		if tNeedle, exists := partialType.Map[ord[0]]; exists && tNeedle != nil {
			typ = tNeedle // solved
		}
		if tHaystack, exists := partialType.Map[ord[1]]; exists && tHaystack != nil {
			if tHaystack.Kind != types.KindList {
				return nil, fmt.Errorf("second arg must be of kind list")
			}
			if typ != nil && typ.Cmp(tHaystack.Val) != nil {
				return nil, fmt.Errorf("list contents in second arg for contains must match search type")
			}
			typ = tHaystack.Val // solved
		}
	}

	if tOut := partialType.Out; tOut != nil {
		if tOut.Kind != types.KindBool {
			return nil, fmt.Errorf("return type must be a bool")
		}
	}

	if typ == nil {
		return variant, nil
	}

	typFunc := types.NewType(fmt.Sprintf("func(needle %s, haystack []%s) bool", typ.String(), typ.String()))

	// TODO: type check that the partialValues are compatible

	return []*types.Type{typFunc}, nil // solved!
}

// Build is run to turn the polymorphic, undeterminted function, into the
// specific statically type version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ContainsPolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return fmt.Errorf("the contains function needs exactly two args")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	tNeedle, exists := typ.Map[typ.Ord[0]]
	if !exists || tNeedle == nil {
		return fmt.Errorf("first arg must be specified")
	}

	tHaystack, exists := typ.Map[typ.Ord[1]]
	if !exists || tHaystack == nil {
		return fmt.Errorf("second arg must be specified")
	}

	if tHaystack.Kind != types.KindList {
		return fmt.Errorf("second argument must be of kind list")
	}

	if err := tHaystack.Val.Cmp(tNeedle); err != nil {
		return errwrap.Wrapf(err, "type of first arg must match type of list elements in second arg")
	}

	if err := typ.Out.Cmp(types.TypeBool); err != nil {
		return errwrap.Wrapf(err, "return type must be a boolean")
	}

	obj.Type = tNeedle // type of value stored in our list
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ContainsPolyFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ContainsPolyFunc) Info() *interfaces.Info {
	typ := types.NewType(fmt.Sprintf("func(needle %s, haystack []%s) bool", obj.Type.String(), obj.Type.String()))
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  typ, // func kind
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ContainsPolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ContainsPolyFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			needle := input.Struct()["needle"]
			haystack := (input.Struct()["haystack"]).(*types.ListValue)

			_, exists := haystack.Contains(needle)
			var result types.Value = &types.BoolValue{V: exists}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *ContainsPolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}

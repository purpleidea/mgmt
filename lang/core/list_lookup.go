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

package core

import (
	"context"
	"fmt"
	"math"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ListLookupFuncName is the name this function is registered as.
	ListLookupFuncName = "list_lookup"

	// arg names...
	listLookupArgNameList  = "list"
	listLookupArgNameIndex = "index"
)

func init() {
	funcs.Register(ListLookupFuncName, func() interfaces.Func { return &ListLookupFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &ListLookupFunc{} // ensure it meets this expectation

// ListLookupFunc is a list index lookup function. If you provide a negative
// index, then it will return the zero value for that type.
type ListLookupFunc struct {
	Type *types.Type // Kind == List, that is used as the list we lookup in

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ListLookupFunc) String() string {
	return ListLookupFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ListLookupFunc) ArgGen(index int) (string, error) {
	seq := []string{listLookupArgNameList, listLookupArgNameIndex}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *ListLookupFunc) sig() *types.Type {
	// func(list []?1, index int, default ?1) ?1
	v := "?1"
	if obj.Type != nil { // don't panic if called speculatively
		v = obj.Type.Val.String()
	}
	return types.NewType(fmt.Sprintf(
		"func(%s []%s, %s int) %s",
		listLookupArgNameList, v,
		listLookupArgNameIndex,
		v,
	))
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ListLookupFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the listlookup function needs exactly two args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	tList, exists := typ.Map[typ.Ord[0]]
	if !exists || tList == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	tIndex, exists := typ.Map[typ.Ord[1]]
	if !exists || tIndex == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}

	if tIndex != nil && tIndex.Kind != types.KindInt {
		return nil, fmt.Errorf("index must be int kind")
	}

	if err := tList.Val.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "return type must match list val type")
	}

	obj.Type = tList // list type
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ListLookupFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindList {
		return fmt.Errorf("type must be a kind of list")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ListLookupFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ListLookupFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ListLookupFunc) Stream(ctx context.Context) error {
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

			l := (input.Struct()[listLookupArgNameList]).(*types.ListValue)
			index := input.Struct()[listLookupArgNameIndex].Int()
			zero := l.Type().Val.New() // the zero value

			// TODO: should we handle overflow by returning zero?
			if index > math.MaxInt { // max int size varies by arch
				return fmt.Errorf("list index overflow, got: %d, max is: %d", index, math.MaxInt)
			}

			// negative index values are "not found" here!
			var result types.Value
			val, exists := l.Lookup(int(index))
			if exists {
				result = val
			} else {
				result = zero
			}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-ctx.Done():
			return nil
		}
	}
}

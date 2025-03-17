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

package corelist

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ListConcatFuncName is the name this function is registered as.
	ListConcatFuncName = "concat" // list.lookup
)

func init() {
	funcs.ModuleRegister(ModuleName, ListConcatFuncName, func() interfaces.Func { return &ListConcatFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &ListConcatFunc{} // ensure it meets this expectation

// ListConcatFunc is a function which combines a number of lists into one list.
// It can take one or more arguments. It combines them in argument order.
type ListConcatFunc struct {
	*wrapped.Func // *wrapped.Func as a type alias to pull in the base impl.

	Type *types.Type // Kind == List, that is used as every list type

	length int // number of arguments

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ListConcatFunc) String() string {
	return ListConcatFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ListConcatFunc) ArgGen(index int) (string, error) {
	return util.NumToAlpha(index), nil
}

// helper
func (obj *ListConcatFunc) sig() *types.Type {
	// func(list []?1, []?1, []?1...) []?1

	if obj.length == 0 { // not yet known
		return nil
	}

	v := "?1"
	if obj.Type != nil { // don't panic if called speculatively
		v = obj.Type.Val.String()
	}

	typ := fmt.Sprintf("[]%s", v) // each arg is this

	repeated := []string{}
	for i := 0; i < obj.length; i++ {
		repeated = append(repeated, typ)
	}
	args := strings.Join(repeated, ", ") // comma separated args

	return types.NewType(fmt.Sprintf("func(%s) %s", args, typ))
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *ListConcatFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	if l := len(partialValues); l < 1 {
		return nil, nil, fmt.Errorf("function must have at least one arg")
	}

	obj.length = len(partialValues) // store for later

	return obj.sig(), []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ListConcatFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if obj.length < 1 {
		return nil, fmt.Errorf("the function needs at least one arg")
	}
	if len(typ.Ord) < 1 {
		return nil, fmt.Errorf("the function needs at least one arg")
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

	for i, x := range typ.Ord {
		if err := typ.Map[x].Cmp(tList); err != nil {
			return nil, errwrap.Wrapf(err, "arg %d must match all the others", i)
		}
	}

	if err := tList.Cmp(typ.Out); err != nil {
		return nil, errwrap.Wrapf(err, "return type must match list type")
	}

	obj.Func = &wrapped.Func{
		Name: obj.String(),
		FuncInfo: &wrapped.Info{
			// TODO: dedup with below Info data
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: true,
		},
		Type: typ, // .Copy(),
	}

	obj.Type = tList // list type
	fn := &types.FuncValue{
		T: typ,
		V: obj.Call, // implementation
	}
	obj.Fn = fn // inside wrapper.Func
	//return obj.Fn.T, nil
	return obj.sig(), nil
}

// Copy is implemented so that the obj.length value is not lost if we copy this
// function. That value is learned during FuncInfer, and previously would have
// been lost by the time we used it in Build.
func (obj *ListConcatFunc) Copy() interfaces.Func {
	return &ListConcatFunc{
		Type:   obj.Type, // don't copy because we use this after unification
		length: obj.length,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ListConcatFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	values := []types.Value{}

	// TODO: Could speculation pass in non-lists here and cause a panic?
	for _, x := range args {
		values = append(values, x.List()...)
	}

	return &types.ListValue{
		T: obj.Type, // aka l.Type()
		V: values,
	}, nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ListConcatFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindList {
		return fmt.Errorf("type must be a kind of list")
	}
	if obj.length == 0 {
		return fmt.Errorf("function not built correctly")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ListConcatFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: true,
		Fast: true,
		Spec: true,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

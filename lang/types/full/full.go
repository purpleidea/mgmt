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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package full

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// FuncValue represents a function value, for example a built-in or a lambda.
//
// In most languages, we can simply call a function with a list of arguments and
// expect to receive a single value. In this language, however, a function might
// be something like datetime.now() or fn(n) {shell(Sprintf("seq %d", n))},
// which might not produce a value immediately, and might then produce multiple
// values over time. Thus, in this language, a FuncValue does not receive
// Values, instead it receives input Func nodes. The FuncValue then adds more
// Func nodes and edges in order to arrange for output values to be sent to a
// particular output node, which the function returns so that the caller may
// connect that output node to more nodes down the line.
//
// The function can also return an error which could represent that something
// went horribly wrong. (Think, an internal panic.)
type FuncValue struct {
	types.Base
	Name     *string
	Timeful  func(interfaces.Txn, []interfaces.Func) (interfaces.Func, error)
	Timeless *types.FuncValue
	T        *types.Type // contains ordered field types, arg names are a bonus part
}

// String returns a visual representation of this value.
func (obj *FuncValue) String() string {
	return fmt.Sprintf("func(%+v)", obj.T) // TODO: can't print obj.V w/o vet warning
}

// Type returns the type data structure that represents this type.
func (obj *FuncValue) Type() *types.Type { return obj.T }

// Less compares to value and returns true if we're smaller. This panics if the
// two types aren't the same.
func (obj *FuncValue) Less(v types.Value) bool {
	panic("functions are not comparable")
}

// Cmp returns an error if this value isn't the same as the arg passed in.
func (obj *FuncValue) Cmp(val types.Value) error {
	if obj == nil || val == nil {
		return fmt.Errorf("cannot cmp to nil")
	}
	if err := obj.Type().Cmp(val.Type()); err != nil {
		return errwrap.Wrapf(err, "cannot cmp types")
	}

	if obj != val { // best we can do
		return fmt.Errorf("different pointers")
	}

	return nil
}

// Copy returns a copy of this value.
func (obj *FuncValue) Copy() types.Value {
	// TODO: can we do something useful here?
	panic("cannot implement Copy() for FuncValue, because FuncValue is a full.FuncValue, not a Value")
}

// Value returns the raw value of this type.
func (obj *FuncValue) Value() interface{} {
	// TODO: can we do something useful here?
	panic("cannot implement Value() for FuncValue, because FuncValue is a full.FuncValue, not a Value")
	//typ := obj.T.Reflect()
	//
	//// wrap our function with the translation that is necessary
	//fn := func(args []reflect.Value) (results []reflect.Value) { // build
	//	innerArgs := []types.Value{}
	//	for _, x := range args {
	//		v, err := types.ValueOf(x) // reflect.Value -> Value
	//		if err != nil {
	//			panic(fmt.Sprintf("can't determine value of %+v", x))
	//		}
	//		innerArgs = append(innerArgs, v)
	//	}
	//	result, err := obj.V(innerArgs) // call it
	//	if err != nil {
	//		// when calling our function with the Call method, then
	//		// we get the error output and have a chance to decide
	//		// what to do with it, but when calling it from within
	//		// a normal golang function call, the error represents
	//		// that something went horribly wrong, aka a panic...
	//		panic(fmt.Sprintf("function panic: %+v", err))
	//	}
	//	return []reflect.Value{reflect.ValueOf(result.Value())} // only one result
	//}
	//val := reflect.MakeFunc(typ, fn)
	//return val.Interface()
}

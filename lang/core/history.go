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

package core // TODO: should this be in its own individual package?

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// HistoryFuncName is the name this function is registered as.
	// TODO: move this into a separate package
	HistoryFuncName = "history"

	// arg names...
	historyArgNameValue = "value"
	historyArgNameIndex = "index"
)

func init() {
	funcs.Register(HistoryFuncName, func() interfaces.Func { return &HistoryFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &HistoryFunc{} // ensure it meets this expectation

// HistoryFunc is special function which returns the Nth oldest value seen. It
// must store up incoming values until it gets enough to return the desired one.
// A restart of the program, will expunge the stored state. This obviously takes
// more memory, the further back you wish to index. A change in the index var is
// generally not useful, but it is permitted. Moving it to a smaller value will
// cause older index values to be expunged. If this is undesirable, a max count
// could be added. This was not implemented with efficiency in mind. Since some
// functions might not send out un-changed values, it might also make sense to
// implement a *time* based hysteresis, since this only looks at the last N
// changed values. A time based hysteresis would tick every precision-width, and
// store whatever the latest value at that time is.
type HistoryFunc struct {
	Type *types.Type // type of input value (same as output type)

	init *interfaces.Init

	history []types.Value // goes from newest (index->0) to oldest (len()-1)

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *HistoryFunc) String() string {
	return HistoryFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *HistoryFunc) ArgGen(index int) (string, error) {
	seq := []string{historyArgNameValue, historyArgNameIndex}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *HistoryFunc) sig() *types.Type {
	// func(value ?1, index int) ?1
	s := "?1"
	if obj.Type != nil {
		s = obj.Type.String()
	}
	return types.NewType(fmt.Sprintf("func(%s %s, %s int) %s", historyArgNameValue, s, historyArgNameIndex, s))
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. That type is used to build our function
// statically.
func (obj *HistoryFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the history function needs exactly two args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	if t1.Cmp(types.TypeInt) != nil {
		return nil, fmt.Errorf("second arg for history must be an int")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	obj.Type = t0 // type of historical value is now known!

	return obj.sig(), nil
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *HistoryFunc) Copy() interfaces.Func {
	return &HistoryFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *HistoryFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *HistoryFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(), // helper
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *HistoryFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *HistoryFunc) Stream(ctx context.Context) error {
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

			//if obj.last != nil && input.Cmp(obj.last) == nil {
			//	continue // value didn't change, skip it
			//}
			//obj.last = input // store for next

			index := int(input.Struct()[historyArgNameIndex].Int())
			value := input.Struct()[historyArgNameValue]
			var result types.Value

			if index < 0 {
				return fmt.Errorf("can't use a negative index of %d", index)
			}

			// 1) truncate history so length equals index
			if len(obj.history) > index {
				// remove all but first N elements, where N == index
				obj.history = obj.history[:index]
			}

			// 2) (un)shift (add our new value to the beginning)
			obj.history = append([]types.Value{value}, obj.history...)

			// 3) are we ready to output a sufficiently old value?
			if index >= len(obj.history) {
				continue // not enough history is stored yet...
			}

			// 4) read one off the back
			result = obj.history[len(obj.history)-1]

			// TODO: do we want to do this?
			// if the result is still the same, skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

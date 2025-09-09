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

package structs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ForKVFuncName is the unique name identifier for this function.
	ForKVFuncName = "forkv"

	// ForKVFuncArgNameMap is the name for the edge which connects the input
	// map to CallFunc.
	ForKVFuncArgNameMap = "map"
)

// ForKVFunc receives a map from upstream. We iterate over the received map to
// build a subgraph that processes each key and val, and in doing so we get a
// larger function graph. This is rebuilt as necessary if the input map changes.
type ForKVFunc struct {
	interfaces.Textarea

	KeyType *types.Type
	ValType *types.Type

	EdgeName string // name of the edge used

	SetOnIterBody func(innerTxn interfaces.Txn, ptr types.Value, key, val interfaces.Func) error
	ClearIterBody func(length int)

	ArgVertices []interfaces.Func // only one expected

	init *interfaces.Init

	lastForKVMap       types.Value // remember the last value
	lastInputMapLength int         // remember the last input map length
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ForKVFunc) String() string {
	return ForKVFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *ForKVFunc) Validate() error {
	if obj.KeyType == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.ValType == nil {
		return fmt.Errorf("must specify a type")
	}

	// TODO: maybe we can remove this if we use this for core functions...
	if obj.EdgeName == "" {
		return fmt.Errorf("must specify an edge name")
	}

	if len(obj.ArgVertices) != 1 {
		return fmt.Errorf("function did not receive shape information")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *ForKVFunc) Info() *interfaces.Info {
	var typ *types.Type

	if obj.KeyType != nil && obj.ValType != nil { // don't panic if called speculatively
		// XXX: Improve function engine so it can return no value?
		//typ = types.NewType(fmt.Sprintf("func(%s map{%s: %s})", obj.EdgeName, obj.KeyType, obj.ValType)) // returns nothing
		// dummy type to prove we're dropping the output since we don't use it.
		typ = types.NewType(fmt.Sprintf("func(%s map{%s: %s}) nil", obj.EdgeName, obj.KeyType, obj.ValType))
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *ForKVFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.lastForKVMap = nil
	obj.lastInputMapLength = -1
	return nil
}

func (obj *ForKVFunc) replaceSubGraph(subgraphInput interfaces.Func) error {
	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	obj.ClearIterBody(obj.lastInputMapLength) // XXX: pass in size?

	forKVMap := obj.lastForKVMap.Map()
	// XXX: Should we loop in a deterministic order?
	// XXX: Should our type support the new iterator pattern?
	for k := range forKVMap {
		ptr := k
		argNameKey := "forkvInputMapKey"
		argNameVal := "forkvInputMapVal"

		// the key
		inputElemFuncKey := SimpleFnToDirectFunc(
			fmt.Sprintf("forkvInputElemKey[%v]", ptr),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("inputElemFuncKey: expected a single argument")
					}
					//arg := args[0]
					//m, ok := arg.(*types.MapValue)
					//if !ok {
					//	return nil, fmt.Errorf("inputElemFuncKey: expected a MapValue argument")
					//}
					// XXX: If we had some sort of index fn?
					//return m.Map().Index(?), nil
					return k, nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argNameKey, obj.mapType(), obj.KeyType)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFuncKey)

		obj.init.Txn.AddEdge(subgraphInput, inputElemFuncKey, &interfaces.FuncEdge{
			Args: []string{argNameKey},
		})

		// the val
		inputElemFuncVal := SimpleFnToDirectFunc(
			fmt.Sprintf("forkvInputElemVal[%v]", ptr),
			&types.FuncValue{
				V: func(_ context.Context, args []types.Value) (types.Value, error) {
					if len(args) != 1 {
						return nil, fmt.Errorf("inputElemFuncVal: expected a single argument")
					}
					//return v, nil // If we always rebuild the map.
					arg := args[0]
					m, ok := arg.(*types.MapValue)
					if !ok {
						return nil, fmt.Errorf("inputElemFuncVal: expected a MapValue argument")
					}
					return m.Map()[ptr], nil
				},
				T: types.NewType(fmt.Sprintf("func(%s %s) %s", argNameVal, obj.mapType(), obj.ValType)),
			},
		)
		obj.init.Txn.AddVertex(inputElemFuncVal)

		obj.init.Txn.AddEdge(subgraphInput, inputElemFuncVal, &interfaces.FuncEdge{
			Args: []string{argNameVal},
		})

		if err := obj.SetOnIterBody(obj.init.Txn, ptr, inputElemFuncKey, inputElemFuncVal); err != nil {
			return errwrap.Wrapf(err, "could not call SetOnIterBody()")
		}
	}

	return obj.init.Txn.Commit()
}

func (obj *ForKVFunc) mapType() *types.Type {
	return types.NewType(fmt.Sprintf("map{%s: %s}", obj.KeyType, obj.ValType))
}

// cmpMapKeys compares the input map with the cached private lastForKVMap field.
// If either are nil, or if the keys of the maps are not identical, then this
// errors.
func (obj *ForKVFunc) cmpMapKeys(m types.Value) error {
	if obj.lastForKVMap == nil || m == nil {
		return fmt.Errorf("got a nil map")
	}

	m1 := obj.lastForKVMap.Map()
	m2 := m.(*types.MapValue) // must not panic!
	if len(m1) != len(m.Map()) {
		return fmt.Errorf("lengths differ")
	}

	for k := range m1 {
		if _, exists := m2.Lookup(k); !exists {
			return fmt.Errorf("key not found")
		}
	}

	return nil
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ForKVFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	forKVMap := args[0]
	n := len(forKVMap.Map())

	// If the keys are the same, that's enough! We don't need to rebuild the
	// graph unless any of the keys/ change, since those are our unique
	// identifiers into the whole loop. As a result, we don't compare
	// between the entire two maps, since while we could rebuild the graph
	// on any change, it's easier to leave it as is and simply push new
	// values down the already built graph if any value changes.
	if obj.lastInputMapLength != n || obj.cmpMapKeys(forKVMap) != nil {
		subgraphInput := obj.ArgVertices[0]

		// TODO: Technically we only need to save keys!
		obj.lastForKVMap = forKVMap
		obj.lastInputMapLength = n
		// replaceSubGraph uses the above two values
		if err := obj.replaceSubGraph(subgraphInput); err != nil {
			return nil, errwrap.Wrapf(err, "could not replace subgraph")
		}

		return nil, interfaces.ErrInterrupt
	}

	// send dummy value to the output
	return types.NewNil(), nil // dummy value
}

// Cleanup runs after that function was removed from the graph.
func (obj *ForKVFunc) Cleanup(ctx context.Context) error {
	return obj.init.Txn.Reverse()
}

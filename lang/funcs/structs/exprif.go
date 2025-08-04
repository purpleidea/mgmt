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
	"sync"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ExprIfFuncName is the unique name identifier for this function.
	ExprIfFuncName = "exprif"

	// ExprIfFuncArgNameCondition is the name for the edge which connects
	// the input condition to ExprIfFunc.
	ExprIfFuncArgNameCondition = "condition"
)

// ExprIfFunc is a function that passes through the value of the correct branch
// based on the conditional value it gets.
type ExprIfFunc struct {
	interfaces.Textarea

	Type *types.Type // this is the type of the if expression output we hold

	EdgeName string // name of the edge used

	ThenGraph *pgraph.Graph
	ElseGraph *pgraph.Graph

	ThenFunc interfaces.Func
	ElseFunc interfaces.Func

	OutputVertex interfaces.Func

	init *interfaces.Init
	last *bool // last value received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ExprIfFunc) String() string {
	return ExprIfFuncName
}

// Validate tells us if the input struct takes a valid form.
func (obj *ExprIfFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}

	if obj.EdgeName == "" {
		return fmt.Errorf("must specify an edge name")
	}

	if obj.ThenFunc == nil {
		return fmt.Errorf("must specify a then func")
	}
	if obj.ElseFunc == nil {
		return fmt.Errorf("must specify an else func")
	}

	t1 := obj.ThenFunc.Info().Sig.Out
	t2 := obj.ElseFunc.Info().Sig.Out
	if err := t1.Cmp(t2); err != nil {
		return errwrap.Wrapf(err, "types of exprif branches must match")
	}

	if obj.OutputVertex == nil {
		return fmt.Errorf("the output vertex is missing")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *ExprIfFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map: map[string]*types.Type{
				ExprIfFuncArgNameCondition: types.TypeBool, // conditional must be a boolean
			},
			Ord: []string{ExprIfFuncArgNameCondition}, // conditional
			Out: obj.Type,                             // result type must match
		}
	}

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this if expression function.
func (obj *ExprIfFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *ExprIfFunc) Stream(ctx context.Context) error {
	// XXX: is there a sync.Once sort of solution that would be more elegant here?
	mutex := &sync.Mutex{}
	done := false
	send := func(ctx context.Context, b bool) error {
		mutex.Lock()
		defer mutex.Unlock()
		if done {
			return nil
		}
		done = true
		defer close(obj.init.Output) // the sender closes

		if !b {
			return nil
		}

		// send dummy value to the output
		select {
		case obj.init.Output <- types.NewFloat(): // XXX: dummy value
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	}
	defer send(ctx, false) // just close

	defer func() {
		obj.init.Txn.Reverse()
	}()

	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // block looping back here
				if !done {
					return fmt.Errorf("input closed without ever sending anything")
				}
				return nil
			}

			value, exists := input.Struct()[obj.EdgeName]
			if !exists {
				return fmt.Errorf("programming error, can't find edge")
			}

			b := value.Bool()

			if obj.last != nil && *obj.last == b {
				continue // result didn't change
			}
			obj.last = &b // store new result

			if err := obj.replaceSubGraph(b); err != nil {
				return errwrap.Wrapf(err, "could not replace subgraph")
			}

			send(ctx, true) // send dummy and then close

			continue

		case <-ctx.Done():
			return nil
		}
	}
}

func (obj *ExprIfFunc) replaceSubGraph(b bool) error {
	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	var f interfaces.Func
	if b {
		obj.init.Txn.AddGraph(obj.ThenGraph)
		f = obj.ThenFunc
	} else {
		obj.init.Txn.AddGraph(obj.ElseGraph)
		f = obj.ElseFunc
	}

	// create the new subgraph
	edgeName := OutputFuncArgName
	edge := &interfaces.FuncEdge{Args: []string{edgeName}}
	obj.init.Txn.AddVertex(f)
	obj.init.Txn.AddEdge(f, obj.OutputVertex, edge)

	return obj.init.Txn.Commit()
}

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
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ExprExceptFuncName is the unique name identifier for this function.
	ExprExceptFuncName = "exprexcept"

	// ExprExceptFuncArgNameValue is the name for the edge which connects
	// the input (primary) value to ExprExceptFunc.
	ExprExceptFuncArgNameValue = "value"
)

// ExprExceptFunc is the catch-point function which implements the except
// operator. It receives the value of the primary expression, and while that
// value is healthy, it wires the primary expression output directly to the
// output vertex. If the primary expression errors with a catchable (sentinel)
// error, the function engine will deliver that error here as a *types.ErrValue
// instead of shutting down, and in response we swap in the (lazily attached)
// except subgraph, so that the fallback value is used instead. If the primary
// expression ever recovers, we tear that subgraph back down and revert to the
// primary value. Since we implement the ExceptableFunc interface, we are the
// one function which receives upstream errors instead of having them propagate
// past us.
type ExprExceptFunc struct {
	interfaces.Textarea

	Type *types.Type // this is the type of the except expression output

	EdgeName string // name of the edge used

	// PrimaryFunc is the vertex which produces the primary value. It is
	// permanently part of the graph (it must keep running so that it can
	// recover) and we wire it to the output vertex when it is healthy.
	PrimaryFunc interfaces.Func

	// ExceptGraph is the pre-built subgraph which produces the fallback
	// value. It is not part of the running graph until an error arrives.
	ExceptGraph *pgraph.Graph

	// ExceptFunc is the vertex in ExceptGraph which produces the fallback
	// value.
	ExceptFunc interfaces.Func

	OutputVertex interfaces.Func

	init *interfaces.Init
	last *bool // last error state received to use for diff
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ExprExceptFunc) String() string {
	return ExprExceptFuncName
}

// Validate tells us if the input struct takes a valid form.
func (obj *ExprExceptFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}

	if obj.EdgeName == "" {
		return fmt.Errorf("must specify an edge name")
	}

	if obj.PrimaryFunc == nil {
		return fmt.Errorf("must specify a primary func")
	}
	if obj.ExceptGraph == nil {
		return fmt.Errorf("must specify an except graph")
	}
	if obj.ExceptFunc == nil {
		return fmt.Errorf("must specify an except func")
	}

	t1 := obj.PrimaryFunc.Info().Sig.Out
	t2 := obj.ExceptFunc.Info().Sig.Out
	if err := t1.Cmp(t2); err != nil {
		return errwrap.Wrapf(err, "types of except halves must match")
	}

	if obj.OutputVertex == nil {
		return fmt.Errorf("the output vertex is missing")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *ExprExceptFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map: map[string]*types.Type{
				obj.EdgeName: obj.Type, // the primary value
			},
			Ord: []string{obj.EdgeName},
			Out: obj.Type, // result type must match
		}
	}

	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this except expression function.
func (obj *ExprExceptFunc) Init(init *interfaces.Init) error {
	obj.init = init
	// If this func is being re-added to the graph, then our previous
	// subgraph was already torn down (via Cleanup/Reverse) when we were
	// removed. Reset last so that the next Call always rebuilds the branch
	// subgraph, otherwise a stale last matching the error state would
	// wrongly skip the rebuild.
	obj.last = nil
	return nil
}

// replaceSubGraph swaps which side feeds the output vertex. When we're in the
// errored state, we attach the except subgraph (this is the first time it
// starts running) and use its value. When we're healthy, that subgraph gets
// torn down, and the primary vertex feeds the output directly.
func (obj *ExprExceptFunc) replaceSubGraph(isErr bool) error {
	// delete the old subgraph
	if err := obj.init.Txn.Reverse(); err != nil {
		return errwrap.Wrapf(err, "could not Reverse")
	}

	f := obj.PrimaryFunc
	if isErr {
		obj.init.Txn.AddGraph(obj.ExceptGraph)
		f = obj.ExceptFunc
	}

	// create the new subgraph
	edgeName := OutputFuncArgName
	edge := &interfaces.FuncEdge{Args: []string{edgeName}}
	obj.init.Txn.AddVertex(f)
	obj.init.Txn.AddEdge(f, obj.OutputVertex, edge)

	return obj.init.Txn.Commit()
}

// Call this func and return the value if it is possible to do so at this time.
func (obj *ExprExceptFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	value := args[0]
	_, isErr := types.IsErr(value) // did the primary expression error?

	if obj.last == nil || *obj.last != isErr {
		obj.last = &isErr // store new result

		if err := obj.replaceSubGraph(isErr); err != nil {
			return nil, errwrap.Wrapf(err, "could not replace subgraph")
		}

		return nil, interfaces.ErrInterrupt
	}

	// send dummy value to the output
	return types.NewNil(), nil // dummy value
}

// Exceptable is a marker method that tells the engine we can receive
// *types.ErrValue inputs in Call.
func (obj *ExprExceptFunc) Exceptable() {}

// Cleanup runs after that function was removed from the graph.
func (obj *ExprExceptFunc) Cleanup(ctx context.Context) error {
	return obj.init.Txn.Reverse()
}

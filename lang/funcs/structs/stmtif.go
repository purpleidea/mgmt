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
	// StmtIfFuncName is the unique name identifier for this function.
	StmtIfFuncName = "stmtif"

	// StmtIfFuncArgNameCondition is the name for the edge which connects
	// the input condition to StmtIfFunc.
	StmtIfFuncArgNameCondition = "condition"
)

// StmtIfFunc is a function that builds the correct body based on the
// conditional value it gets.
type StmtIfFunc struct {
	interfaces.Textarea

	EdgeName string // name of the edge used

	// Env is the captured environment from when Graph for StmtIf was built.
	Env *interfaces.Env

	// Then is the Stmt for the "then" branch. We do *not* want this to be a
	// *pgraph.Graph, as we actually need to call Graph(env) ourself during
	// runtime to get the correct subgraph out of that appropriate branch.
	Then interfaces.Stmt

	// Else is the Stmt for the "else" branch. See "Then" for more details.
	Else interfaces.Stmt

	init         *interfaces.Init
	last         *bool // last value received to use for diff
	needsReverse bool
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *StmtIfFunc) String() string {
	return StmtIfFuncName
}

// Validate tells us if the input struct takes a valid form.
func (obj *StmtIfFunc) Validate() error {
	if obj.EdgeName == "" {
		return fmt.Errorf("must specify an edge name")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *StmtIfFunc) Info() *interfaces.Info {
	// dummy type to prove we're dropping the output since we don't use it.
	typ := types.NewType(fmt.Sprintf("func(%s bool) nil", obj.EdgeName))

	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this if statement function.
func (obj *StmtIfFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

func (obj *StmtIfFunc) replaceSubGraph(b bool) error {
	if obj.needsReverse { // not on the first run
		// delete the old subgraph
		if err := obj.init.Txn.Reverse(); err != nil {
			return errwrap.Wrapf(err, "could not Reverse")
		}
	}
	obj.needsReverse = true

	if b && obj.Then != nil {
		g, err := obj.Then.Graph(obj.Env)
		if err != nil {
			return err
		}
		obj.init.Txn.AddGraph(g)
	}
	if !b && obj.Else != nil {
		g, err := obj.Else.Graph(obj.Env)
		if err != nil {
			return err
		}
		obj.init.Txn.AddGraph(g)
	}

	return obj.init.Txn.Commit()
}

// Call this func and return the value if it is possible to do so at this time.
func (obj *StmtIfFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	value := args[0]
	b := value.Bool()

	if obj.last == nil || *obj.last != b {
		obj.last = &b // store new result

		if err := obj.replaceSubGraph(b); err != nil {
			return nil, errwrap.Wrapf(err, "could not replace subgraph")
		}

		return nil, interfaces.ErrInterrupt
	}

	// send dummy value to the output
	return types.NewNil(), nil // dummy value
}

// Cleanup runs after that function was removed from the graph.
func (obj *StmtIfFunc) Cleanup(ctx context.Context) error {
	if !obj.needsReverse { // not needed if we never replaced graph
		return nil
	}

	return obj.init.Txn.Reverse()
}

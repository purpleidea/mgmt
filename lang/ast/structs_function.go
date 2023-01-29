// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

//package structs
package ast

import (
	"fmt"

	//"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// FunctionFunc is a function that passes through the function body it receives.
type FunctionFunc struct {
	Type *types.Type // this is the type of the function that we hold
	Edge string      // name of the edge used (typically "body")
	Func interfaces.Func
	Fn   *types.SimpleFn

	init   *interfaces.Init
	last   types.Value // last value received to use for diff
	result types.Value // last calculated output

	closeChan chan struct{}
}

// fn returns the function that wraps the Func interface if that API is used.
func (obj *FunctionFunc) fn() (*types.SimpleFn, error) {
	panic("TODO [SimpleFn]: should this use FuncValue or SimpleFn?")
	//fn := func(args []types.Value) (types.Value, error) {
	//	// FIXME: can we run a recursive engine
	//	// to support running non-pure functions?
	//	if !obj.Func.Info().Pure {
	//		return nil, fmt.Errorf("only pure functions can be used by value")
	//	}

	//	// XXX: this might not be needed anymore...
	//	return funcs.PureFuncExec(obj.Func, args)
	//}

	//result := types.NewFunc(obj.Type) // new func
	//if err := result.Set(fn); err != nil {
	//	return nil, errwrap.Wrapf(err, "can't build func from built-in")
	//}

	//return result, nil
}

// Validate makes sure we've built our struct properly.
func (obj *FunctionFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("must specify a type")
	}
	if obj.Type.Kind != types.KindFunc {
		return fmt.Errorf("can't use type `%s`", obj.Type.String())
	}
	if obj.Edge == "" && obj.Func == nil && obj.Fn == nil {
		return fmt.Errorf("must specify an Edge, Func, or Fn")
	}

	if obj.Fn != nil && obj.Fn.Type() != obj.Type {
		return fmt.Errorf("type of Fn did not match input Type")
	}

	return nil
}

// Info returns some static info about itself.
func (obj *FunctionFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		typ = &types.Type{
			Kind: types.KindFunc, // function type
			Map:  make(map[string]*types.Type),
			Ord:  []string{},
			Out:  obj.Type, // after the function is called it's this...
		}

		// type of body is what we'd get by running the function (what's inside)
		if obj.Edge != "" {
			typ.Map[obj.Edge] = obj.Type.Out
			typ.Ord = append(typ.Ord, obj.Edge)
		}
	}

	pure := true // assume true
	if obj.Func != nil {
		pure = obj.Func.Info().Pure
	}

	return &interfaces.Info{
		Pure: pure,  // TODO: can we guarantee this?
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this composite function.
func (obj *FunctionFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream takes an input struct in the format as described in the Func and Graph
// methods of the Expr, and returns the actual expected value as a stream based
// on the changing inputs to that value.
func (obj *FunctionFunc) Stream() error {
	// TODO:
	//
	// In all three cases, we produce a single FuncValue and then close the
	// stream to indicate that the value will not change anymore.
	//
	// If the function represents a lambda (Edge != nil):
	//   The body of lambdas are no longer inlined at the call sites, so this
	//   case is no longer possible. Instead, a Lambda is compiled to a node
	//   with in-degree zero (could be a FunctionFunc, could be a new node
	//   type called a ClosureFunc) which produces a single FuncValue, which
	//   in turn takes the lambda's arguments and produces a sub-graph
	//   consisting of the lambda's body.
	//   Note that in
	//
	//       fn(x) {
	//         fn (y) {
	//           x + y
	//         }
	//       }
	//
	//   the inner lambda's node does not need to have the x node as an input,
	//   because when x changes, it does not cause the shape of the sub-graph to
	//   change, it merely changes which values flow through the plus node.
	//   However, the inner lambda's FuncValue does need to store the x node, so
	//   that it can generate a sub-graph in which the plus node is connected to
	//   that x node.
	// If the function represents ? (Func != nil):
	//   Return Func.
	// If the function represents a go function (Fn != nil):
	//   Return a FuncValue which constructs a subgraph consisting of a single
	//   node (let's call it PureFunc) which runs Fn.
	panic("TODO [SimpleFn]: see comment above")
	//defer close(obj.init.Output) // the sender closes
	//for {
	//	select {
	//	case input, ok := <-obj.init.Input:
	//		if !ok {
	//			if obj.Edge != "" { // then it's not a built-in
	//				return nil // can't output any more
	//			}

	//			var result *types.SimpleFn

	//			if obj.Fn != nil {
	//				result = obj.Fn
	//			} else {
	//				var err error
	//				result, err = obj.fn()
	//				if err != nil {
	//					return err
	//				}
	//			}

	//			// if we never had input args, send the function
	//			select {
	//			case obj.init.Output <- result: // send
	//				// pass
	//			case <-obj.closeChan:
	//				return nil
	//			}

	//			return nil
	//		}
	//		//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
	//		//	return errwrap.Wrapf(err, "wrong function input")
	//		//}
	//		if obj.last != nil && input.Cmp(obj.last) == nil {
	//			continue // value didn't change, skip it
	//		}
	//		obj.last = input // store for next

	//		var result types.Value

	//		st := input.(*types.StructValue)     // must be!
	//		value, exists := st.Lookup(obj.Edge) // single argName
	//		if !exists {
	//			return fmt.Errorf("missing expected input argument `%s`", obj.Edge)
	//		}

	//		result = obj.Type.New() // new func
	//		fn := func([]types.Value) (types.Value, error) {
	//			return value, nil
	//		}
	//		if err := result.(*types.SimpleFn).Set(fn); err != nil {
	//			return errwrap.Wrapf(err, "can't build func with body")
	//		}

	//		// skip sending an update...
	//		if obj.result != nil && result.Cmp(obj.result) == nil {
	//			continue // result didn't change
	//		}
	//		obj.result = result // store new result

	//	case <-obj.closeChan:
	//		return nil
	//	}

	//	select {
	//	case obj.init.Output <- obj.result: // send
	//		// pass
	//	case <-obj.closeChan:
	//		return nil
	//	}
	//}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *FunctionFunc) Close() error {
	close(obj.closeChan)
	return nil
}

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

package funcs // this is here, in case we allow others to register operators...

import (
	"fmt"
	"math"
	"sort"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

const (
	// OperatorFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	OperatorFuncName = "_operator"

	// operatorArgName is the edge and arg name used for the function's operator.
	operatorArgName = "x" // something short and arbitrary
)

func init() {
	// concatenation
	RegisterOperator("+", &types.FuncValue{
		T: types.NewType("func(a str, b str) str"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.StrValue{
				V: input[0].Str() + input[1].Str(),
			}, nil
		},
	})
	// addition
	RegisterOperator("+", &types.FuncValue{
		T: types.NewType("func(a int, b int) int"),
		V: func(input []types.Value) (types.Value, error) {
			//if l := len(input); l != 2 {
			//	return nil, fmt.Errorf("expected two inputs, got: %d", l)
			//}
			// FIXME: check for overflow?
			return &types.IntValue{
				V: input[0].Int() + input[1].Int(),
			}, nil
		},
	})
	// floating-point addition
	RegisterOperator("+", &types.FuncValue{
		T: types.NewType("func(a float, b float) float"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.FloatValue{
				V: input[0].Float() + input[1].Float(),
			}, nil
		},
	})

	// subtraction
	RegisterOperator("-", &types.FuncValue{
		T: types.NewType("func(a int, b int) int"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.IntValue{
				V: input[0].Int() - input[1].Int(),
			}, nil
		},
	})
	// floating-point subtraction
	RegisterOperator("-", &types.FuncValue{
		T: types.NewType("func(a float, b float) float"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.FloatValue{
				V: input[0].Float() - input[1].Float(),
			}, nil
		},
	})

	// multiplication
	RegisterOperator("*", &types.FuncValue{
		T: types.NewType("func(a int, b int) int"),
		V: func(input []types.Value) (types.Value, error) {
			// FIXME: check for overflow?
			return &types.IntValue{
				V: input[0].Int() * input[1].Int(),
			}, nil
		},
	})
	// floating-point multiplication
	RegisterOperator("*", &types.FuncValue{
		T: types.NewType("func(a float, b float) float"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.FloatValue{
				V: input[0].Float() * input[1].Float(),
			}, nil
		},
	})

	// don't add: `func(int, float) float` or: `func(float, int) float`
	// division
	RegisterOperator("/", &types.FuncValue{
		T: types.NewType("func(a int, b int) float"),
		V: func(input []types.Value) (types.Value, error) {
			divisor := input[1].Int()
			if divisor == 0 {
				return nil, fmt.Errorf("can't divide by zero")
			}
			return &types.FloatValue{
				V: float64(input[0].Int()) / float64(divisor),
			}, nil
		},
	})
	// floating-point division
	RegisterOperator("/", &types.FuncValue{
		T: types.NewType("func(a float, b float) float"),
		V: func(input []types.Value) (types.Value, error) {
			divisor := input[1].Float()
			if divisor == 0.0 {
				return nil, fmt.Errorf("can't divide by zero")
			}
			return &types.FloatValue{
				V: input[0].Float() / divisor,
			}, nil
		},
	})

	// string equality
	RegisterOperator("==", &types.FuncValue{
		T: types.NewType("func(a str, b str) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Str() == input[1].Str(),
			}, nil
		},
	})
	// bool equality
	RegisterOperator("==", &types.FuncValue{
		T: types.NewType("func(a bool, b bool) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() == input[1].Bool(),
			}, nil
		},
	})
	// int equality
	RegisterOperator("==", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() == input[1].Int(),
			}, nil
		},
	})
	// floating-point equality
	RegisterOperator("==", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() == input[1].Float(),
			}, nil
		},
	})

	// string in-equality
	RegisterOperator("!=", &types.FuncValue{
		T: types.NewType("func(a str, b str) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Str() == input[1].Str(),
			}, nil
		},
	})
	// bool in-equality
	RegisterOperator("!=", &types.FuncValue{
		T: types.NewType("func(a bool, b bool) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() != input[1].Bool(),
			}, nil
		},
	})
	// int in-equality
	RegisterOperator("!=", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() != input[1].Int(),
			}, nil
		},
	})
	// floating-point in-equality
	RegisterOperator("!=", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() != input[1].Float(),
			}, nil
		},
	})

	// less-than
	RegisterOperator("<", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() < input[1].Int(),
			}, nil
		},
	})
	// floating-point less-than
	RegisterOperator("<", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() < input[1].Float(),
			}, nil
		},
	})
	// greater-than
	RegisterOperator(">", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() > input[1].Int(),
			}, nil
		},
	})
	// floating-point greater-than
	RegisterOperator(">", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() > input[1].Float(),
			}, nil
		},
	})
	// less-than-equal
	RegisterOperator("<=", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() <= input[1].Int(),
			}, nil
		},
	})
	// floating-point less-than-equal
	RegisterOperator("<=", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() <= input[1].Float(),
			}, nil
		},
	})
	// greater-than-equal
	RegisterOperator(">=", &types.FuncValue{
		T: types.NewType("func(a int, b int) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Int() >= input[1].Int(),
			}, nil
		},
	})
	// floating-point greater-than-equal
	RegisterOperator(">=", &types.FuncValue{
		T: types.NewType("func(a float, b float) bool"),
		V: func(input []types.Value) (types.Value, error) {
			// TODO: should we do an epsilon check?
			return &types.BoolValue{
				V: input[0].Float() >= input[1].Float(),
			}, nil
		},
	})

	// logical and
	// TODO: is there a way for the engine to have
	// short-circuit operators, and does it matter?
	RegisterOperator("&&", &types.FuncValue{
		T: types.NewType("func(a bool, b bool) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() && input[1].Bool(),
			}, nil
		},
	})
	// logical or
	RegisterOperator("||", &types.FuncValue{
		T: types.NewType("func(a bool, b bool) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() || input[1].Bool(),
			}, nil
		},
	})

	// logical not (unary operator)
	RegisterOperator("!", &types.FuncValue{
		T: types.NewType("func(a bool) bool"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: !input[0].Bool(),
			}, nil
		},
	})

	// pi operator (this is an easter egg to demo a zero arg operator)
	RegisterOperator("π", &types.FuncValue{
		T: types.NewType("func() float"),
		V: func(input []types.Value) (types.Value, error) {
			return &types.FloatValue{
				V: math.Pi,
			}, nil
		},
	})

	Register(OperatorFuncName, func() interfaces.Func { return &OperatorPolyFunc{} }) // must register the func and name
}

// OperatorFuncs maps an operator to a list of callable function values.
var OperatorFuncs = make(map[string][]*types.FuncValue) // must initialize

// RegisterOperator registers the given string operator and function value
// implementation with the mini-database for this generalized, static,
// polymorphic operator implementation.
func RegisterOperator(operator string, fn *types.FuncValue) {
	if _, exists := OperatorFuncs[operator]; !exists {
		OperatorFuncs[operator] = []*types.FuncValue{} // init
	}

	for _, f := range OperatorFuncs[operator] {
		if err := f.T.Cmp(fn.T); err == nil {
			panic(fmt.Sprintf("operator %s already has an implementation for %+v", operator, f.T))
		}
	}

	for i, x := range fn.T.Ord {
		if x == operatorArgName {
			panic(fmt.Sprintf("can't use `%s` as an argName for operator `%s` with type `%+v`", x, operator, fn.T))
		}
		// yes this limits the arg max to 24 (`x`) including operator
		if s := util.NumToAlpha(i); x != s {
			panic(fmt.Sprintf("arg for operator `%s` (index `%d`) should be named `%s`, not `%s`", operator, i, s, x))
		}
	}

	OperatorFuncs[operator] = append(OperatorFuncs[operator], fn)
}

// LookupOperator returns a list of type strings for each operator. An empty
// operator string means return everything. If you specify a size that is less
// than zero, we don't filter by arg length, otherwise we only return signatures
// which have an arg length equal to size.
func LookupOperator(operator string, size int) ([]*types.Type, error) {
	fns, exists := OperatorFuncs[operator]
	if !exists && operator != "" {
		return nil, fmt.Errorf("operator not found")
	}
	results := []*types.Type{}

	if operator == "" {
		var keys []string
		for k := range OperatorFuncs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, a := range keys {
			fns = append(fns, OperatorFuncs[a]...)
		}
	}

	for _, fn := range fns {
		typ := addOperatorArg(fn.T)        // add in the `operatorArgName` arg
		typ = unlabelOperatorArgNames(typ) // label in standard a..b..c

		if size >= 0 && len(typ.Ord) != size {
			continue
		}
		results = append(results, typ)
	}

	return results, nil
}

// OperatorPolyFunc is an operator function that performs an operation on N
// values.
type OperatorPolyFunc struct {
	Type *types.Type // Kind == Function, including operator arg

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output

	closeChan chan struct{}
}

// argNames returns the maximum list of possible argNames. This can be truncated
// if needed. The first arg name is the operator.
func (obj *OperatorPolyFunc) argNames() []string {
	// we could just do this statically, but i did it dynamically so that I
	// wouldn't ever have to remember to update this list...
	max := 0
	for _, fns := range OperatorFuncs {
		for _, fn := range fns {
			l := len(fn.T.Ord)
			if l > max {
				max = l
			}
		}
	}
	//if length >= 0 && length < max {
	//	max = length
	//}

	args := []string{operatorArgName}
	for i := 0; i < max; i++ {
		s := util.NumToAlpha(i)
		if s == operatorArgName {
			panic(fmt.Sprintf("can't use `%s` as arg name", operatorArgName))
		}
		args = append(args, s)
	}

	return args
}

// findFunc tries to find the first available registered operator function that
// matches the Operator/Type pattern requested. If none is found it returns nil.
func (obj *OperatorPolyFunc) findFunc(operator string) *types.FuncValue {
	fns, exists := OperatorFuncs[operator]
	if !exists {
		return nil
	}
	typ := removeOperatorArg(obj.Type) // remove operator so we can match...
	for _, fn := range fns {
		if err := fn.Type().Cmp(typ); err == nil { // found one!
			return fn
		}
	}
	return nil
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *OperatorPolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	var op string
	var size = -1

	// optimization: if operator happens to already be known statically,
	// then we can return a much smaller subset of possible signatures...
	if partialType != nil && partialType.Ord != nil {
		ord := partialType.Ord
		if len(ord) == 0 {
			return nil, fmt.Errorf("must have at least one arg in operator func")
		}
		// optimization: since we know arg length, we can limit the
		// signatures that we return...
		size = len(ord) // we know size!
		if partialType.Map != nil {
			if t, exists := partialType.Map[ord[0]]; exists && t != nil {
				if t.Cmp(types.TypeStr) != nil {
					return nil, fmt.Errorf("first arg for operator func must be an str")
				}
				if len(partialValues) > 0 && partialValues[0] != nil {
					op = partialValues[0].Str() // known str
				}
			}
		}
	}

	// since built-in functions have their functions explicitly defined, we
	// can add easy invariants between in/out args and their expected types.
	results, err := LookupOperator(op, size)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error findings signatures for operator `%s`", op)
	}

	// TODO: we can add additional results filtering here if we'd like...

	if len(results) == 0 {
		return nil, fmt.Errorf("no matching signatures for operator `%s` could be found", op)
	}

	return results, nil
}

// Build is run to turn the polymorphic, undeterminted function, into the
// specific statically type version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs. It typically re-labels the input arg names to match what is actually
// used.
func (obj *OperatorPolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...

	if len(typ.Ord) < 1 {
		return fmt.Errorf("the operator function needs at least 1 arg")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}

	t, err := obj.relabelOperatorArgNames(typ)
	if err != nil {
		return fmt.Errorf("could not build function from type: %+v", typ)
	}
	obj.Type = t // func type
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *OperatorPolyFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	if obj.Type.Kind != types.KindFunc {
		return fmt.Errorf("type must be a kind of func")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *OperatorPolyFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.Type, // func kind, which includes operator arg as input
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this fact.
func (obj *OperatorPolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *OperatorPolyFunc) Stream() error {
	var op, lastOp string
	var fn *types.FuncValue
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

			// build up arg list
			args := []types.Value{}
			for _, name := range obj.Type.Ord {
				v := input.Struct()[name]
				if name == operatorArgName {
					op = v.Str()
					continue // skip over the operator arg
				}
				args = append(args, v)
			}

			if op == "" {
				return fmt.Errorf("operator cannot be empty")
			}
			// operator selection is dynamic now, although mostly it
			// should not change... to do so is probably uncommon...
			if fn == nil || op != lastOp {
				fn = obj.findFunc(op)
			}
			if fn == nil {
				return fmt.Errorf("func not found for operator `%s` with sig: `%+v`", op, obj.Type)
			}
			lastOp = op

			var result types.Value
			result, err := fn.Call(args) // run the function
			if err != nil {
				return errwrap.Wrapf(err, "problem running function")
			}
			if result == nil {
				return fmt.Errorf("computed function output was nil")
			}

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

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *OperatorPolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}

// relabelOperatorArgNames relabels the input type of kind func with arg names
// that match the expected ones for this operator (which are all standardized).
func (obj *OperatorPolyFunc) relabelOperatorArgNames(typ *types.Type) (*types.Type, error) {
	if typ == nil {
		return nil, fmt.Errorf("cannot re-label missing type")
	}
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("specified type must be a func kind")
	}

	argNames := obj.argNames() // correct arg names...

	if l := len(argNames); len(typ.Ord) > l {
		return nil, fmt.Errorf("did not expect more than %d args", l)
	}

	m := make(map[string]*types.Type)
	ord := []string{}
	for pos, x := range typ.Ord { // function args in order
		name := argNames[pos] // new arg name
		m[name] = typ.Map[x]  // n-th type stored with new arg name
		ord = append(ord, name)
	}
	return &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  typ.Out,
	}, nil
}

// unlabelOperatorArgNames unlabels the input type of kind func with arg names
// that match the default ones for all functions (which are all standardized).
func unlabelOperatorArgNames(typ *types.Type) *types.Type {
	if typ == nil {
		return nil
	}

	m := make(map[string]*types.Type)
	ord := []string{}
	for pos, x := range typ.Ord { // function args in order
		name := util.NumToAlpha(pos) // default (unspecified) naming
		m[name] = typ.Map[x]         // n-th type stored with new arg name
		ord = append(ord, name)
	}
	return &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  typ.Out,
	}
}

// removeOperatorArg returns a copy of the input KindFunc type, without the
// operator arg which specifies which operator we're using. It *is* idempotent.
func removeOperatorArg(typ *types.Type) *types.Type {
	if typ == nil {
		return nil
	}
	if _, exists := typ.Map[operatorArgName]; !exists {
		return typ // pass through
	}

	m := make(map[string]*types.Type)
	ord := []string{}
	for _, s := range typ.Ord {
		if s == operatorArgName {
			continue // remove the operator
		}
		m[s] = typ.Map[s]
		ord = append(ord, s)
	}
	return &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  typ.Out,
	}
}

// addOperatorArg returns a copy of the input KindFunc type, with the operator
// arg which specifies which operator we're using added. This is idempotent.
func addOperatorArg(typ *types.Type) *types.Type {
	if typ == nil {
		return nil
	}
	if _, exists := typ.Map[operatorArgName]; exists {
		return typ // pass through
	}

	m := make(map[string]*types.Type)
	m[operatorArgName] = types.TypeStr // add the operator
	ord := []string{operatorArgName}   // add the operator
	for _, s := range typ.Ord {
		m[s] = typ.Map[s]
		ord = append(ord, s)
	}
	return &types.Type{
		Kind: types.KindFunc,
		Map:  m,
		Ord:  ord,
		Out:  typ.Out,
	}
}

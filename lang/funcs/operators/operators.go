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

// Package operators provides a helper library to load all of the built-in
// operators, which are actually just functions.
package operators // this is here, in case we allow others to register operators

import (
	"context"
	"fmt"
	"math"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// OperatorFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	OperatorFuncName = "_operator"

	// operatorArgName is the edge and arg name used for the function's
	// operator.
	operatorArgName = "op" // something short and arbitrary
)

func init() {
	RegisterOperator("+", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) ?1"),
		C: simple.TypeMatch([]string{
			"func(str, str) str",       // concatenation
			"func(int, int) int",       // addition
			"func(float, float) float", // floating-point addition
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindStr:
				return &types.StrValue{
					V: input[0].Str() + input[1].Str(),
				}, nil

			case types.KindInt:
				// FIXME: check for overflow?
				return &types.IntValue{
					V: input[0].Int() + input[1].Int(),
				}, nil

			case types.KindFloat:
				return &types.FloatValue{
					V: input[0].Float() + input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator("-", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) ?1"),
		C: simple.TypeMatch([]string{
			"func(int, int) int",       // subtraction
			"func(float, float) float", // floating-point subtraction
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				return &types.IntValue{
					V: input[0].Int() - input[1].Int(),
				}, nil

			case types.KindFloat:
				return &types.FloatValue{
					V: input[0].Float() - input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator("*", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) ?1"),
		C: simple.TypeMatch([]string{
			"func(int, int) int",       // multiplication
			"func(float, float) float", // floating-point multiplication
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				// FIXME: check for overflow?
				return &types.IntValue{
					V: input[0].Int() * input[1].Int(),
				}, nil

			case types.KindFloat:
				return &types.FloatValue{
					V: input[0].Float() * input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	// don't add: `func(int, float) float` or: `func(float, int) float`
	RegisterOperator("/", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) float"),
		C: simple.TypeMatch([]string{
			"func(int, int) float",     // division
			"func(float, float) float", // floating-point division
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				divisor := input[1].Int()
				if divisor == 0 {
					return nil, fmt.Errorf("can't divide by zero")
				}
				return &types.FloatValue{
					V: float64(input[0].Int()) / float64(divisor),
				}, nil

			case types.KindFloat:
				divisor := input[1].Float()
				if divisor == 0.0 {
					return nil, fmt.Errorf("can't divide by zero")
				}
				return &types.FloatValue{
					V: input[0].Float() / divisor,
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator("==", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: func(typ *types.Type) error {
			//if typ == nil { // happens within iter
			//	return fmt.Errorf("nil type")
			//}
			iterFn := func(typ *types.Type) error {
				if typ == nil {
					return fmt.Errorf("nil type")
				}
				if !types.IsComparableKind(typ.Kind) {
					return fmt.Errorf("not comparable")
				}
				return nil
			}
			if err := types.Iter(typ, iterFn); err != nil {
				return err
			}

			// At this point, we know we can cmp any contained type.
			match := simple.TypeMatch([]string{
				//"func(bool, bool) bool",             // bool equality
				//"func(str, str) bool",               // string equality
				//"func(int, int) bool",               // int equality
				//"func(float, float) bool",           // floating-point equality
				//"func([]?1, []?1) bool",             // list equality
				//"func(map{?1:?2}, map{?1:?2}) bool", // map equality
				// struct in-equality (just skip the entire match function)
				"func(?1, ?1) bool",
			})
			return match(typ)
		},
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			k := input[0].Type().Kind
			// Don't try and compare functions, this will panic!
			if !types.IsComparableKind(k) {
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}

			return &types.BoolValue{
				V: input[0].Cmp(input[1]) == nil, // equality
			}, nil
		},
	})

	RegisterOperator("!=", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: func(typ *types.Type) error {
			//if typ == nil { // happens within iter
			//	return fmt.Errorf("nil type")
			//}
			iterFn := func(typ *types.Type) error {
				if typ == nil {
					return fmt.Errorf("nil type")
				}
				if !types.IsComparableKind(typ.Kind) {
					return fmt.Errorf("not comparable")
				}
				return nil
			}
			if err := types.Iter(typ, iterFn); err != nil {
				return err
			}

			// At this point, we know we can cmp any contained type.
			match := simple.TypeMatch([]string{
				//"func(bool, bool) bool",             // bool in-equality
				//"func(str, str) bool",               // string in-equality
				//"func(int, int) bool",               // int in-equality
				//"func(float, float) bool",           // floating-point in-equality
				//"func([]?1, []?1) bool",             // list in-equality
				//"func(map{?1:?2}, map{?1:?2}) bool", // map in-equality
				// struct in-equality (just skip the entire match function)
				"func(?1, ?1) bool",
			})
			return match(typ)
		},
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			k := input[0].Type().Kind
			// Don't try and compare functions, this will panic!
			if !types.IsComparableKind(k) {
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}

			return &types.BoolValue{
				V: input[0].Cmp(input[1]) != nil, // in-equality
			}, nil
		},
	})

	RegisterOperator("<", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: simple.TypeMatch([]string{
			"func(int, int) bool",     // less-than
			"func(float, float) bool", // floating-point less-than
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				return &types.BoolValue{
					V: input[0].Int() < input[1].Int(),
				}, nil

			case types.KindFloat:
				// TODO: should we do an epsilon check?
				return &types.BoolValue{
					V: input[0].Float() < input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator(">", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: simple.TypeMatch([]string{
			"func(int, int) bool",     // greater-than
			"func(float, float) bool", // floating-point greater-than
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				return &types.BoolValue{
					V: input[0].Int() > input[1].Int(),
				}, nil

			case types.KindFloat:
				// TODO: should we do an epsilon check?
				return &types.BoolValue{
					V: input[0].Float() > input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator("<=", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: simple.TypeMatch([]string{
			"func(int, int) bool",     // less-than-equal
			"func(float, float) bool", // floating-point less-than-equal
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				return &types.BoolValue{
					V: input[0].Int() <= input[1].Int(),
				}, nil

			case types.KindFloat:
				// TODO: should we do an epsilon check?
				return &types.BoolValue{
					V: input[0].Float() <= input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	RegisterOperator(">=", &simple.Scaffold{
		T: types.NewType("func(?1, ?1) bool"),
		C: simple.TypeMatch([]string{
			"func(int, int) bool",     // greater-than-equal
			"func(float, float) bool", // floating-point greater-than-equal
		}),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			switch k := input[0].Type().Kind; k {
			case types.KindInt:
				return &types.BoolValue{
					V: input[0].Int() >= input[1].Int(),
				}, nil

			case types.KindFloat:
				// TODO: should we do an epsilon check?
				return &types.BoolValue{
					V: input[0].Float() >= input[1].Float(),
				}, nil

			default:
				return nil, fmt.Errorf("unsupported kind: %+v", k)
			}
		},
	})

	// logical and
	// TODO: is there a way for the engine to have
	// short-circuit operators, and does it matter?
	RegisterOperator("and", &simple.Scaffold{
		T: types.NewType("func(bool, bool) bool"),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() && input[1].Bool(),
			}, nil
		},
	})

	// logical or
	RegisterOperator("or", &simple.Scaffold{
		T: types.NewType("func(bool, bool) bool"),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: input[0].Bool() || input[1].Bool(),
			}, nil
		},
	})

	// logical not (unary operator)
	RegisterOperator("not", &simple.Scaffold{
		T: types.NewType("func(bool) bool"),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			return &types.BoolValue{
				V: !input[0].Bool(),
			}, nil
		},
	})

	// pi operator (this is an easter egg to demo a zero arg operator)
	RegisterOperator("π", &simple.Scaffold{
		T: types.NewType("func() float"),
		F: func(ctx context.Context, input []types.Value) (types.Value, error) {
			return &types.FloatValue{
				V: math.Pi,
			}, nil
		},
	})

	// register a copy in the main function database
	// XXX: use simple.Register instead?
	funcs.Register(OperatorFuncName, func() interfaces.Func { return &OperatorFunc{} })
}

var _ interfaces.InferableFunc = &OperatorFunc{} // ensure it meets this expectation

// OperatorFuncs maps an operator to a list of callable function values.
var OperatorFuncs = make(map[string]*simple.Scaffold) // must initialize

// RegisterOperator registers the given string operator and function value
// implementation with the mini-database for this generalized, static,
// polymorphic operator implementation.
func RegisterOperator(operator string, scaffold *simple.Scaffold) {
	if _, exists := OperatorFuncs[operator]; exists {
		panic(fmt.Sprintf("operator %s already has an implementation", operator))
	}

	if scaffold == nil {
		panic(fmt.Sprintf("no scaffold specified for operator %s", operator))
	}
	if scaffold.T == nil {
		panic(fmt.Sprintf("no type specified for operator %s", operator))
	}
	if scaffold.T.Kind != types.KindFunc {
		panic(fmt.Sprintf("operator %s type must be a func", operator))
	}
	if scaffold.T.HasVariant() {
		panic(fmt.Sprintf("operator %s contains a variant type signature", operator))
	}
	// It's okay if scaffold.C is nil.
	if scaffold.F == nil {
		panic(fmt.Sprintf("no implementation specified for operator %s", operator))
	}

	for _, x := range scaffold.T.Ord {
		if x == operatorArgName {
			panic(fmt.Sprintf("can't use `%s` as an argName for operator `%s` with type `%+v`", x, operator, scaffold.T))
		}
		// yes this limits the arg max to 24 (`x`) including operator
		// if the operator is `x`...
		//if s := util.NumToAlpha(i); x != s {
		//	panic(fmt.Sprintf("arg for operator `%s` (index `%d`) should be named `%s`, not `%s`", operator, i, s, x))
		//}
	}

	OperatorFuncs[operator] = scaffold // store a copy for ourselves
}

// LookupOperator returns the type for the operator you looked up. It errors if
// it doesn't exist, or if the arg length isn't equal to size.
func LookupOperator(operator string, size int) (*types.Type, error) {
	scaffold, exists := OperatorFuncs[operator]
	if !exists {
		return nil, fmt.Errorf("operator not found")
	}

	typ := addOperatorArg(scaffold.T) // add in the `operatorArgName` arg
	if len(typ.Ord) != size {
		return nil, fmt.Errorf("operator has wrong size")
	}

	return typ, nil
}

// OperatorFunc is an operator function that performs an operation on N values.
// XXX: Can we wrap SimpleFunc instead of having the boilerplate here ourselves?
type OperatorFunc struct {
	*docsUtil.Metadata

	Type *types.Type // Kind == Function, including operator arg

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *OperatorFunc) String() string {
	// TODO: return the exact operator if we can guarantee it doesn't change
	return OperatorFuncName
}

// argNames returns the maximum list of possible argNames. This can be truncated
// if needed. The first arg name is the operator.
func (obj *OperatorFunc) argNames() ([]string, error) {
	// we could just do this statically, but i did it dynamically so that I
	// wouldn't ever have to remember to update this list...
	m := 0 // max
	for _, scaffold := range OperatorFuncs {
		m = max(m, len(scaffold.T.Ord))
	}

	args := []string{operatorArgName}
	for i := 0; i < m; i++ {
		s := util.NumToAlpha(i)
		if s == operatorArgName {
			return nil, fmt.Errorf("can't use `%s` as arg name", operatorArgName)
		}
		args = append(args, s)
	}

	return args, nil
}

// findFunc tries to find the first available registered operator function that
// matches the Operator/Type pattern requested. If none is found it returns nil.
func (obj *OperatorFunc) findFunc(operator string) interfaces.FuncSig {
	scaffold, exists := OperatorFuncs[operator]
	if !exists {
		return nil
	}
	//typ := removeOperatorArg(obj.Type) // remove operator so we can match...
	//for _, fn := range fns {
	//	if err := fn.Type().Cmp(typ); err == nil { // found one!
	//		return fn
	//	}
	//}
	return scaffold.F
}

// ArgGen returns the Nth arg name for this function.
func (obj *OperatorFunc) ArgGen(index int) (string, error) {
	seq, err := obj.argNames()
	if err != nil {
		return "", err
	}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *OperatorFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// The operator must be known statically to be able to return a result.
	if partialType == nil || len(partialValues) == 0 {
		return nil, nil, fmt.Errorf("partials must not be nil or empty")
	}
	// redundant
	//if partialType.Map == nil || len(partialType.Map) == 0 {
	//	return nil, nil, fmt.Errorf("must have at least one arg in operator func")
	//}
	//if partialType.Ord == nil || len(partialType.Ord) == 0 {
	//	return nil, nil, fmt.Errorf("must have at least one arg in operator func")
	//}

	val := partialValues[0]
	if val == nil {
		return nil, nil, fmt.Errorf("first arg for operator func must not be nil")
	}

	if err := val.Type().Cmp(types.TypeStr); err != nil { // op must be str
		return nil, nil, fmt.Errorf("first arg for operator func must be an str")
	}
	op := val.Str()              // known str
	size := len(partialType.Ord) // we know size!

	typ, err := LookupOperator(op, size)
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "error finding signature for operator `%s`", op)
	}
	if typ == nil {
		return nil, nil, fmt.Errorf("no matching signature for operator `%s` could be found", op)
	}

	return typ, []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *OperatorFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if len(typ.Ord) < 1 {
		return nil, fmt.Errorf("the operator function needs at least 1 arg")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("unexpected build kind of: %v", typ.Kind)
	}

	// Change arg names to be what we expect...
	if _, exists := typ.Map[typ.Ord[0]]; !exists {
		return nil, fmt.Errorf("invalid build type")
	}

	//newTyp := typ.Copy()
	newTyp := &types.Type{
		Kind: typ.Kind,                     // copy
		Map:  make(map[string]*types.Type), // new
		Ord:  []string{},                   // new
		Out:  typ.Out,                      // copy
	}
	for i, x := range typ.Ord { // remap arg names
		//argName := util.NumToAlpha(i - 1)
		//if i == 0 {
		//	argName = operatorArgName
		//}
		argName, err := obj.ArgGen(i)
		if err != nil {
			return nil, err
		}

		newTyp.Map[argName] = typ.Map[x]
		newTyp.Ord = append(newTyp.Ord, argName)
	}

	obj.Type = newTyp // func type
	return obj.Type, nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *OperatorFunc) Validate() error {
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
func (obj *OperatorFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.Type, // func kind, which includes operator arg as input
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *OperatorFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *OperatorFunc) Stream(ctx context.Context) error {
	var op, lastOp string
	var fn interfaces.FuncSig
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

			// programming error safety check...
			programmingError := false
			keys := []string{}
			for k := range input.Struct() {
				keys = append(keys, k)
				if !util.StrInList(k, obj.Type.Ord) {
					programmingError = true
				}
			}
			if programmingError {
				return fmt.Errorf("bad args, got: %v, want: %v", keys, obj.Type.Ord)
			}

			// build up arg list
			args := []types.Value{}
			for _, name := range obj.Type.Ord {
				v, exists := input.Struct()[name]
				if !exists {
					// programming error
					return fmt.Errorf("function engine was early, missing arg: %s", name)
				}
				if name == operatorArgName {
					op = v.Str()
					continue // skip over the operator arg
				}
				args = append(args, v)
			}

			if op == "" {
				// programming error
				return fmt.Errorf("operator cannot be empty, args: %v", keys)
			}
			// operator selection is dynamic now, although mostly it
			// should not change... to do so is probably uncommon...
			if fn == nil {
				fn = obj.findFunc(op)

			} else if op != lastOp {
				// TODO: check sig is compatible instead?
				return fmt.Errorf("op changed from %s to %s", lastOp, op)
			}

			if fn == nil {
				return fmt.Errorf("func not found for operator `%s` with sig: `%+v`", op, obj.Type)
			}
			lastOp = op

			var result types.Value

			result, err := fn(ctx, args) // (Value, error)
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

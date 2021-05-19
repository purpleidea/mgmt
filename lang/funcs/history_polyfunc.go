// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package funcs // TODO: should this be in its own individual package?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// HistoryFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	HistoryFuncName = "_history"
)

func init() {
	Register(HistoryFuncName, func() interfaces.Func { return &HistoryFunc{} }) // must register the func and name
}

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

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *HistoryFunc) ArgGen(index int) (string, error) {
	seq := []string{"value", "index"}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *HistoryFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(value T1, index int) T1

	valueName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	indexName, err := obj.ArgGen(1)
	if err != nil {
		return nil, err
	}

	dummyValue := &interfaces.ExprAny{} // corresponds to the value type
	dummyIndex := &interfaces.ExprAny{} // corresponds to the index type
	dummyOut := &interfaces.ExprAny{}   // corresponds to the out string

	// index arg type of int
	invar = &interfaces.EqualsInvariant{
		Expr: dummyIndex,
		Type: types.TypeInt,
	}
	invariants = append(invariants, invar)

	// index and return are the same type
	invar = &interfaces.EqualityInvariant{
		Expr1: dummyValue,
		Expr2: dummyOut,
	}
	invariants = append(invariants, invar)

	// full function
	mapped := make(map[string]interfaces.Expr)
	ordered := []string{valueName, indexName}
	mapped[valueName] = dummyValue
	mapped[indexName] = dummyIndex

	invar = &interfaces.EqualityWrapFuncInvariant{
		Expr1:    expr, // maps directly to us!
		Expr2Map: mapped,
		Expr2Ord: ordered,
		Expr2Out: dummyOut,
	}
	invariants = append(invariants, invar)

	// generator function
	fn := func(fnInvariants []interfaces.Invariant, solved map[interfaces.Expr]*types.Type) ([]interfaces.Invariant, error) {
		for _, invariant := range fnInvariants {
			// search for this special type of invariant
			cfavInvar, ok := invariant.(*interfaces.CallFuncArgsValueInvariant)
			if !ok {
				continue
			}
			// did we find the mapping from us to ExprCall ?
			if cfavInvar.Func != expr {
				continue
			}
			// cfavInvar.Expr is the ExprCall! (the return pointer)
			// cfavInvar.Args are the args that ExprCall uses!
			if l := len(cfavInvar.Args); l != 2 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOut,
			}
			invariants = append(invariants, invar)

			// second arg must be an int
			invar = &interfaces.EqualsInvariant{
				Expr: cfavInvar.Args[1],
				Type: types.TypeInt,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyValue,
			}
			invariants = append(invariants, invar)
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[1],
				Expr2: dummyIndex,
			}
			invariants = append(invariants, invar)

			if typ, err := cfavInvar.Args[1].Type(); err == nil { // is it known?
				if k := typ.Kind; k != types.KindInt {
					return nil, fmt.Errorf("unable to build function with 1st arg of kind: %s", k)
				}
			}

			// We just need to figure out one type to know the full
			// type...
			var t1 *types.Type // the value type

			// validateArg0 checks: value T1
			validateArg0 := func(typ *types.Type) error {
				if typ == nil { // unknown so far
					return nil
				}

				if err := typ.Cmp(t1); t1 != nil && err != nil {
					return errwrap.Wrapf(err, "input type was inconsistent")
				}

				// learn!
				t1 = typ
				return nil
			}

			if typ, err := cfavInvar.Args[0].Type(); err == nil { // is it known?
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}
			if typ, exists := solved[cfavInvar.Args[0]]; exists { // alternate way to lookup type
				// this sets t1 and t2 on success if it learned
				if err := validateArg0(typ); err != nil {
					return nil, errwrap.Wrapf(err, "first struct arg type is inconsistent")
				}
			}

			// XXX: if the struct type/value isn't know statically?

			if t1 != nil {
				invar = &interfaces.EqualsInvariant{
					Expr: dummyValue,
					Type: t1,
				}
				invariants = append(invariants, invar)

				invar = &interfaces.EqualsInvariant{ // bonus
					Expr: dummyOut,
					Type: t1,
				}
				invariants = append(invariants, invar)
			}

			// TODO: do we return this relationship with ExprCall?
			invar = &interfaces.EqualityWrapCallInvariant{
				// TODO: should Expr1 and Expr2 be reversed???
				Expr1: cfavInvar.Expr,
				//Expr2Func: cfavInvar.Func, // same as below
				Expr2Func: expr,
			}
			invariants = append(invariants, invar)

			// TODO: are there any other invariants we should build?
			return invariants, nil // generator return
		}
		// We couldn't tell the solver anything it didn't already know!
		return nil, fmt.Errorf("couldn't generate new invariants")
	}
	invar = &interfaces.GeneratorInvariant{
		Func: fn,
	}
	invariants = append(invariants, invar)

	return invariants, nil
}

// Polymorphisms returns the possible type signature for this function. In this
// case, since the number of possible types for the first arg can be infinite,
// it returns the final precise type only if it can be gleamed statically. If
// not, it returns that unknown as a variant, which is hopefully solved during
// unification.
func (obj *HistoryFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as first & return arg for now -- maybe there's a better way?
	variant := []*types.Type{types.NewType("func(value variant, index int) variant")}

	if partialType == nil {
		return variant, nil
	}

	var typ *types.Type // = nil is implied

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 2 {
			return nil, fmt.Errorf("must have at exactly two args in history func")
		}
		if t, exists := partialType.Map[ord[1]]; exists && t != nil {
			if t.Cmp(types.TypeInt) != nil {
				return nil, fmt.Errorf("second arg for history must be an int")
			}
		}

		if t, exists := partialType.Map[ord[0]]; exists && t != nil && partialType.Out != nil {
			if t.Cmp(partialType.Out) != nil {
				return nil, fmt.Errorf("type of first arg for history must match return type")
			}
			typ = t // it has been found :)
		}
	}

	if partialType.Out != nil {
		typ = partialType.Out // it has been found :)
	}

	if typ == nil {
		return variant, nil
	}

	t := types.NewType(fmt.Sprintf("func(value %s, index int) %s", typ.String(), typ.String()))

	return []*types.Type{t}, nil // return a list with a single possibility
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. That type is used to build our function
// statically.
func (obj *HistoryFunc) Build(typ *types.Type) error {
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) != 2 {
		return fmt.Errorf("the history function needs exactly two args")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return fmt.Errorf("second arg must be specified")
	}
	if t1.Cmp(types.TypeInt) != nil {
		return fmt.Errorf("second arg for history must be an int")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return fmt.Errorf("first arg must be specified")
	}
	obj.Type = t0 // type of historical value is now known!

	return nil
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
	var sig *types.Type
	if obj.Type != nil { // don't panic if called speculatively
		s := obj.Type.String()
		sig = types.NewType(fmt.Sprintf("func(value %s, index int) %s", s, s))
	}
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *HistoryFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *HistoryFunc) Stream() error {
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

			index := int(input.Struct()["index"].Int())
			value := input.Struct()["value"]
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

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *HistoryFunc) Close() error {
	close(obj.closeChan)
	return nil
}

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

package corefmt

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	// FIXME: should this be named sprintf instead?
	funcs.ModuleRegister(ModuleName, "printf", func() interfaces.Func { return &PrintfFunc{} })
}

const (
	formatArgName = "format" // name of the first arg
)

// PrintfFunc is a static polymorphic function that compiles a format string and
// returns the output as a string. It bases its output on the values passed in
// to it. It examines the type of the arguments at compile time and then
// determines the static function signature by parsing the format string and
// using that to determine the final function signature. One consequence of this
// is that the format string must be a static string which is known at compile
// time. This is reasonable, because if it was a reactive, changing string, then
// we could expect the type signature to change, which is not allowed in our
// statically typed language.
type PrintfFunc struct {
	Type *types.Type // final full type of our function

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result *string // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *PrintfFunc) ArgGen(index int) (string, error) {
	if index == 0 {
		return formatArgName, nil
	}
	// TODO: if index is big enough that it would return the string in
	// `formatArgName` then we should return an error! (Nearly impossible.)
	return util.NumToAlpha(index - 1), nil
}

// Unify returns the list of invariants that this func produces.
func (obj *PrintfFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(format string, args... variant) string

	formatName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	dummyFormat := &interfaces.ExprAny{} // corresponds to the format type
	dummyOut := &interfaces.ExprAny{}    // corresponds to the out string

	// format arg type of string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyFormat,
		Type: types.TypeStr,
	}
	invariants = append(invariants, invar)

	// return type of string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyOut,
		Type: types.TypeStr,
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
			if len(cfavInvar.Args) == 0 {
				return nil, fmt.Errorf("unable to build function with no args")
			}

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationship to the returned value
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Expr,
				Expr2: dummyOut,
			}
			invariants = append(invariants, invar)

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyFormat,
			}
			invariants = append(invariants, invar)

			// first arg must be a string
			invar = &interfaces.EqualsInvariant{
				Expr: cfavInvar.Args[0],
				Type: types.TypeStr,
			}
			invariants = append(invariants, invar)

			// XXX: We could add an alternate mode for this
			// function where instead of knowing args[0]
			// statically, if we happen to know all of the input
			// arg types, we build the function, without verifying
			// that the format string is valid... In this case, if
			// it was built dynamically or happened to not be in
			// the right format, we'd just print out some yucky
			// result. The golang printf does something similar
			// when it can't catch things statically at compile
			// time.
			// XXX: In the above scenario, we'd have to also change
			// the compileFormatToString function to handle a list
			// of values with a badly matched string. Maybe best to
			// just not allow this entirely? Or set this behaviour
			// with a constant?

			value, err := cfavInvar.Args[0].Value() // is it known?
			if err != nil {
				return nil, fmt.Errorf("format string is not known statically")
			}

			if k := value.Type().Kind; k != types.KindStr {
				return nil, fmt.Errorf("unable to build function with 0th arg of kind: %s", k)
			}
			format := value.Str() // must not panic
			typList, err := parseFormatToTypeList(format)
			if err != nil {
				return nil, errwrap.Wrapf(err, "could not parse format string")
			}

			// full function
			mapped := make(map[string]interfaces.Expr)
			ordered := []string{formatName}
			mapped[formatName] = dummyFormat

			for i, x := range typList {
				argName, err := obj.ArgGen(i + 1) // skip 0th
				if err != nil {
					return nil, err
				}
				if argName == formatArgName {
					return nil, fmt.Errorf("could not build function with %d args", i+1) // +1 for format arg
				}

				dummyArg := &interfaces.ExprAny{}
				// if it's a variant, we can't add the invariant
				if x != types.TypeVariant {
					invar = &interfaces.EqualsInvariant{
						Expr: dummyArg,
						Type: x,
					}
					invariants = append(invariants, invar)
				}

				// add the relationships to the called args
				invar = &interfaces.EqualityInvariant{
					Expr1: cfavInvar.Args[i+1],
					Expr2: dummyArg,
				}
				invariants = append(invariants, invar)

				mapped[argName] = dummyArg
				ordered = append(ordered, argName)
			}

			invar = &interfaces.EqualityWrapFuncInvariant{
				Expr1:    expr, // maps directly to us!
				Expr2Map: mapped,
				Expr2Ord: ordered,
				Expr2Out: dummyOut,
			}
			invariants = append(invariants, invar)

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
// case, since the number of arguments can be infinite, it returns the final
// precise type if it can be gleamed from the format argument. If it cannot, it
// is because either the format argument was not known statically, or because it
// had an invalid format string.
// XXX: This version of the function does not handle any variants returned from
// the parseFormatToTypeList helper function.
func (obj *PrintfFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	if partialType == nil || len(partialValues) < 1 {
		return nil, fmt.Errorf("first argument must be a static format string")
	}

	if partialType.Out != nil && partialType.Out.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("return value of printf must be str")
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) < 1 {
			return nil, fmt.Errorf("must have at least one arg in printf func")
		}
		if t, exists := partialType.Map[ord[0]]; exists && t != nil {
			if t.Cmp(types.TypeStr) != nil {
				return nil, fmt.Errorf("first arg for printf must be an str")
			}
		}
	}

	// FIXME: we'd like to pre-compute the interpolation if we can, so that
	// we can run this code properly... for now, we can't, so it's a compile
	// time error...
	if partialValues[0] == nil {
		return nil, fmt.Errorf("could not determine type from format string")
	}

	format := partialValues[0].Str() // must not panic
	typList, err := parseFormatToTypeList(format)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not parse format string")
	}

	typ := &types.Type{
		Kind: types.KindFunc, // function type
		Map:  make(map[string]*types.Type),
		Ord:  []string{},
		Out:  types.TypeStr,
	}
	// add first arg
	typ.Map[formatArgName] = types.TypeStr
	typ.Ord = append(typ.Ord, formatArgName)

	for i, x := range typList {
		name := util.NumToAlpha(i) // start with a...
		if name == formatArgName {
			return nil, fmt.Errorf("could not build function with %d args", i+1) // +1 for format arg
		}

		// if we also had even more partial type information, check it!
		if t, exists := partialType.Map[ord[i+1]]; exists && t != nil {
			if err := t.Cmp(x); err != nil {
				return nil, errwrap.Wrapf(err, "arg %d does not match expected type", i+1)
			}
		}

		typ.Map[name] = x
		typ.Ord = append(typ.Ord, name)
	}

	return []*types.Type{typ}, nil // return a list with a single possibility
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. That type is used to build our function
// statically.
func (obj *PrintfFunc) Build(typ *types.Type) error {
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) < 1 {
		return fmt.Errorf("the printf function needs at least one arg")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Out.Cmp(types.TypeStr) != nil {
		return fmt.Errorf("return type of function must be an str")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return fmt.Errorf("first arg must be specified")
	}
	if t0.Cmp(types.TypeStr) != nil {
		return fmt.Errorf("first arg for printf must be an str")
	}

	obj.Type = typ // function type is now known!
	return nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *PrintfFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *PrintfFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.Type,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *PrintfFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *PrintfFunc) Stream() error {
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

			format := input.Struct()[formatArgName].Str()
			values := []types.Value{}
			for _, name := range obj.Type.Ord {
				if name == formatArgName { // skip format arg
					continue
				}
				x := input.Struct()[name]
				values = append(values, x)
			}

			result, err := compileFormatToString(format, values)
			if err != nil {
				return err // no errwrap needed b/c helper func
			}

			if obj.result != nil && *obj.result == result {
				continue // result didn't change
			}
			obj.result = &result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: *obj.result,
		}:
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *PrintfFunc) Close() error {
	close(obj.closeChan)
	return nil
}

// valueToString prints our values how we expect for printf.
// FIXME: if this turns out to be useful, add it to the types package.
func valueToString(value types.Value) string {
	switch x := value.Type().Kind; x {
	// FIXME: floats don't print nicely: https://github.com/golang/go/issues/46118
	case types.KindFloat:
		// TODO: use formatting flags ?
		// FIXME: Our String() method in FloatValue doesn't print nicely
		return value.String()
	}

	// FIXME: this is just an "easy-out" implementation for now...
	return fmt.Sprintf("%v", value.Value())

	//switch x := value.Type().Kind; x {
	//case types.KindBool:
	//	return value.String()
	//case types.KindStr:
	//	return value.Str() // use this since otherwise it adds " & "
	//case types.KindInt:
	//	return value.String()
	//case types.KindFloat:
	//	// TODO: use formatting flags ?
	//	return value.String()
	//}
	//panic("unhandled type") // TODO: not fully implemented yet
}

// parseFormatToTypeList takes a format string and returns a list of types that
// it expects to use in the order found in the format string. This can also
// handle the %v special variant type in the format string.
// FIXME: add support for more types, and add tests!
func parseFormatToTypeList(format string) ([]*types.Type, error) {
	typList := []*types.Type{}
	inType := false
	for i := 0; i < len(format); i++ {

		// some normal char...
		if !inType && format[i] != '%' {
			continue
		}

		// in a type or we're a %
		if format[i] == '%' {
			if inType {
				// it's a %%
				inType = false
			} else {
				// start looking for type specification!
				inType = true
			}
			continue
		}

		// we must be in a type
		switch format[i] {
		case 't':
			typList = append(typList, types.TypeBool)
		case 's':
			typList = append(typList, types.TypeStr)
		case 'd':
			typList = append(typList, types.TypeInt)

		// TODO: parse fancy formats like %0.2f and stuff
		case 'f':
			typList = append(typList, types.TypeFloat)

		// FIXME: add fancy types like: %[]s, %[]f, %{s:f}, etc...

		// special!
		case 'v':
			typList = append(typList, types.TypeVariant)

		default:
			return nil, fmt.Errorf("invalid format string at %d", i)
		}
		inType = false // done
	}

	return typList, nil
}

// compileFormatToString takes a format string and a list of values and returns
// the compiled/templated output. This can also handle the %v special variant
// type in the format string. Of course the corresponding value to those %v
// entries must have a static, fixed, precise type.
// FIXME: add support for more types, and add tests!
func compileFormatToString(format string, values []types.Value) (string, error) {
	output := ""
	ix := 0
	inType := false
	for i := 0; i < len(format); i++ {

		// some normal char...
		if !inType && format[i] != '%' {
			output += string(format[i])
			continue
		}

		// in a type or we're a %
		if format[i] == '%' {
			if inType {
				// it's a %%
				output += string(format[i])
				inType = false
			} else {
				// start looking for type specification!
				inType = true
			}
			continue
		}

		// we must be in a type
		var typ *types.Type
		switch format[i] {
		case 't':
			typ = types.TypeBool
		case 's':
			typ = types.TypeStr
		case 'd':
			typ = types.TypeInt

		// TODO: parse fancy formats like %0.2f and stuff
		case 'f':
			typ = types.TypeFloat

		// FIXME: add fancy types like: %[]s, %[]f, %{s:f}, etc...

		case 'v':
			typ = types.TypeVariant

		default:
			return "", fmt.Errorf("invalid format string at %d", i)
		}
		inType = false // done

		// check the type (if not a variant) matches what we have...
		if typ == types.TypeVariant {
			if values[ix].Type() == nil {
				return "", fmt.Errorf("unexpected nil type")
			}
		} else if err := typ.Cmp(values[ix].Type()); err != nil {
			return "", errwrap.Wrapf(err, "unexpected type")
		}

		output += valueToString(values[ix])
		ix++ // consume one value
	}

	return output, nil
}

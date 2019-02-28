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

package corefmt

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
)

func init() {
	// FIXME: should this be named sprintf instead?
	funcs.ModuleRegister(moduleName, "printf", func() interfaces.Func { return &PrintfFunc{} })
}

const (
	// XXX: does this need to be `a` ? -- for now yes, fix this compiler bug
	//formatArgName = "format" // name of the first arg
	formatArgName = "a" // name of the first arg
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

	result string // last calculated output

	closeChan chan struct{}
}

// Polymorphisms returns the possible type signature for this function. In this
// case, since the number of arguments can be infinite, it returns the final
// precise type if it can be gleamed from the format argument. If it cannot, it
// is because either the format argument was not known statically, or because
// it had an invalid format string.
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
		name := util.NumToAlpha(i + 1) // +1 to skip the format arg
		if name == formatArgName {
			return nil, fmt.Errorf("could not build function with %d args", i+1)
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

			if obj.result == result {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: obj.result,
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
// it expects to use in the order found in the format string.
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

		default:
			return nil, fmt.Errorf("invalid format string at %d", i)
		}
		inType = false // done
	}

	return typList, nil
}

// compileFormatToString takes a format string and a list of values and returns
// the compiled/templated output.
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

		default:
			return "", fmt.Errorf("invalid format string at %d", i)
		}
		inType = false // done

		if err := typ.Cmp(values[ix].Type()); err != nil {
			return "", errwrap.Wrapf(err, "unexpected type")
		}

		output += valueToString(values[ix])
		ix++ // consume one value
	}

	return output, nil
}

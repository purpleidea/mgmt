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

package corefmt

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	unificationUtil "github.com/purpleidea/mgmt/lang/unification/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// PrintfFuncName is the name this function is registered as.
	// FIXME: should this be named sprintf instead?
	PrintfFuncName = "printf"

	// PrintfAllowNonStaticFormat allows us to use printf when the zeroth
	// argument (the format string) is not known statically at compile time.
	// The downside of this is that if it changes while we are running, it
	// could change from "hello %s" to "hello %d" or "%s %d...". If this
	// happens we can generate ugly format strings, instead of preventing it
	// from even running at all. The behaviour if this happens is determined
	// by PrintfAllowFormatError.
	//
	// NOTE: It's useful to allow dynamic strings if we were generating
	// custom log messages (for example) where the format comes from a
	// database lookup or similar. Of course if we knew that such a lookup
	// could be done quickly and statically (maybe it's a read from a local
	// key-value config file that's part of our deploy) then maybe we can do
	// it before unification speculatively.
	PrintfAllowNonStaticFormat = true

	// PrintfAllowFormatError will cause the function to shutdown if it has
	// an invalid format string. Otherwise this will cause the output of the
	// function to return a garbled message. This is similar to golang's
	// format errors, eg: https://pkg.go.dev/fmt#hdr-Format_errors
	PrintfAllowFormatError = true

	printfArgNameFormat = "format" // name of the first arg
)

func init() {
	funcs.ModuleRegister(ModuleName, PrintfFuncName, func() interfaces.Func { return &PrintfFunc{} })
}

var _ interfaces.InferableFunc = &PrintfFunc{} // ensure it meets this expectation

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

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *PrintfFunc) String() string {
	if obj.Type != nil {
		return fmt.Sprintf("%s: %s", PrintfFuncName, obj.Type)
	}
	return PrintfFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *PrintfFunc) ArgGen(index int) (string, error) {
	if index == 0 {
		return printfArgNameFormat, nil
	}
	// TODO: if index is big enough that it would return the string in
	// `printfArgNameFormat` then we should return an error! (Nearly impossible.)
	return util.NumToAlpha(index - 1), nil
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *PrintfFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(format str, args... variant) string

	if len(partialValues) < 1 {
		return nil, nil, fmt.Errorf("must have at least one arg")
	}
	if len(partialType.Map) < 1 {
		// programming error?
		return nil, nil, fmt.Errorf("must have at least one arg")
	}
	if typ := partialType.Map[partialType.Ord[0]]; typ != nil && typ.Cmp(types.TypeStr) != nil && !typ.HasUni() {
		return nil, nil, fmt.Errorf("format string was a %s", typ)
	}

	getType := func(i int) *types.Type { // get Nth type, doesn't bound check
		if partialValues[i] != nil {
			// We don't check that this is consistent with
			// partialType, because that's a compiler job.
			return partialValues[i].Type() // got it!
		}
		if partialType == nil || partialType.Map == nil {
			return nil // no more information
		}
		return partialType.Map[partialType.Ord[i]]
	}

	getFormat := func() *string {
		if partialValues[0] == nil {
			return nil // no more information
		}
		typ := partialValues[0].Type()
		if typ == nil || typ.Cmp(types.TypeStr) != nil {
			return nil // no more information
		}

		formatString := partialValues[0].Str()
		return &formatString
	}

	typList := make([]*types.Type, len(partialValues)) // number of args at call site

	for i := range partialValues { // populate initial expected types
		typList[i] = getType(i) // nil if missing
	}

	// Do we have type information from the format string? (If it exists!)
	if format := getFormat(); format != nil {
		// formatList doesn't contain zeroth arg in our typList!
		formatList, err := parseFormatToTypeList(*format)
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "could not parse format string")
		}
		if a, l := len(typList)-1, len(formatList); a != l {
			return nil, nil, fmt.Errorf("number of args (%d) doesn't match format string verb count (%d)", a, l)
		}
		for i, x := range typList {
			if i == 0 { // format string
				continue
			}
			if x == nil { // nothing to check against
				typList[i] = formatList[i-1] // use this!
				continue
			}

			// NOTE: Is it okay to allow unification variables here?
			//if x.HasUni() {
			//	// programming error (did the compiler change?)
			//	return nil, nil, fmt.Errorf("programming error at arg index %d", i)
			//}
			if err := unificationUtil.UnifyCmp(x, formatList[i-1]); err != nil {
				return nil, nil, errwrap.Wrapf(err, "inconsistent type at arg index %d", i)
			}
			// Less general version of the above...
			//if err := x.Cmp(formatList[i-1]); err != nil {
			//	return nil, nil, errwrap.Wrapf(err, "inconsistent type at arg index %d", i)
			//}
		}
	} else if !PrintfAllowNonStaticFormat {
		return nil, nil, fmt.Errorf("format string is not known statically")
	}

	// Check the format string is consistent with what we've found earlier!
	if i := 0; typList[i] != nil && typList[i].Cmp(types.TypeStr) != nil && !typList[i].HasUni() {
		return nil, nil, fmt.Errorf("inconsistent type at arg index %d (format string)", i)
	}
	typList[0] = types.TypeStr // format string (zeroth arg)

	mapped := map[string]*types.Type{}
	ordered := []string{}

	for i, x := range typList {
		argName, err := obj.ArgGen(i)
		if err != nil {
			return nil, nil, err
		}

		//if x.HasVariant() {
		//	x = x.VariantToUni() // converts %[]v style things
		//}
		if x == nil || x == types.TypeVariant { // a %v or something unknown
			x = &types.Type{
				Kind: types.KindUnification,
				Uni:  types.NewElem(), // unification variable, eg: ?1
			}
		}

		mapped[argName] = x
		ordered = append(ordered, argName)
	}

	typ := &types.Type{ // this full function
		Kind: types.KindFunc,
		Map:  mapped,
		Ord:  ordered,
		Out:  types.TypeStr,
	}

	return typ, []*interfaces.UnificationInvariant{}, nil
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. That type is used to build our function
// statically.
func (obj *PrintfFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) < 1 {
		return nil, fmt.Errorf("the printf function needs at least one arg")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Out.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("return type of function must be an str")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	t0, exists := typ.Map[typ.Ord[0]]
	if !exists || t0 == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if t0.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("first arg for printf must be an str")
	}

	//newTyp := typ.Copy()
	newTyp := &types.Type{
		Kind: typ.Kind,                     // copy
		Map:  make(map[string]*types.Type), // new
		Ord:  []string{},                   // new
		Out:  typ.Out,                      // copy
	}
	for i, x := range typ.Ord { // remap arg names
		argName, err := obj.ArgGen(i)
		if err != nil {
			return nil, err
		}
		newTyp.Map[argName] = typ.Map[x]
		newTyp.Ord = append(newTyp.Ord, argName)
	}

	obj.Type = newTyp // function type is now known!
	return obj.Type, nil
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
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	return &interfaces.Info{
		Pure: true,
		Memo: true,
		Fast: true,
		Spec: true,
		Sig:  obj.Type,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *PrintfFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *PrintfFunc) Stream(ctx context.Context) error {
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

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}

			result, err := obj.Call(ctx, args)
			if err != nil {
				return err
			}

			// if the result is still the same, skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

// Copy is implemented so that the obj.Type value is not lost if we copy this
// function.
func (obj *PrintfFunc) Copy() interfaces.Func {
	return &PrintfFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *PrintfFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	format := args[0].Str()

	values := []types.Value{}
	for i, x := range args {
		if i == 0 { // skip format arg
			continue
		}
		values = append(values, x)
	}

	result, err := compileFormatToString(format, values)
	if err != nil {
		return nil, err // no errwrap needed b/c helper func
	}
	return &types.StrValue{
		V: result,
	}, nil
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

	case types.KindList:
		fallthrough
	case types.KindMap:
		fallthrough
	case types.KindStruct:
		// XXX: Attempting to run value.Value() on a struct, which will
		// surely have a lowercase (unexported) name will panic. This
		// will also happen on any container types like list or map that
		// may have a struct inside. It's expected that we have
		// lowercase field names since we're using mcl, but workaround
		// this panic for now with this cheap representation.
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
			//typList = append(typList, types.TypeVariant) // old
			typList = append(typList, types.NewType("?1")) // uni!

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
// entries must have a static, fixed, precise type. If someone changes the
// format string during runtime, then that's their fault, and this could error.
// Depending on PrintfAllowFormatError, we should NOT error if we have a
// mismatch between the format string and the available args. Return similar to
// golang's EXTRA/MISSING, eg: https://pkg.go.dev/fmt#hdr-Format_errors
// TODO: implement better format errors support
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
			// TODO: improve the output of this
			if !PrintfAllowFormatError {
				return fmt.Sprintf("<invalid format `%v` at %d>", format[i], i), nil
			}
			return "", fmt.Errorf("invalid format string at %d", i)
		}
		inType = false // done

		if ix >= len(values) {
			// TODO: improve the output of this
			if !PrintfAllowFormatError {
				return fmt.Sprintf("<invalid format length `%d` at %d>", ix, i), nil
			}
			return "", fmt.Errorf("more specifiers (%d) than values (%d)", ix+1, len(values))
		}

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

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

package coregolang

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"text/template"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// TemplateFuncName is the name this function is registered as.
	TemplateFuncName = "template"

	// TemplateName is the name of our template as required by the template
	// library.
	TemplateName = "template"

	// arg names...
	templateArgNameTemplate = "template"
	templateArgNameVars     = "vars"
)

var (
	// errorType represents a reflection type of error as seen in:
	// https://github.com/golang/go/blob/ec62ee7f6d3839fe69aeae538dadc1c9dc3bf020/src/text/template/exec.go#L612
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

func init() {
	funcs.ModuleRegister(ModuleName, TemplateFuncName, func() interfaces.Func { return &TemplateFunc{} })
}

var _ interfaces.InferableFunc = &TemplateFunc{} // ensure it meets this expectation

// TemplateFunc is a static polymorphic function that compiles a template and
// returns the output as a string. It bases its output on the values passed in
// to it. It examines the type of the second argument (the input data vars) at
// compile time and then determines the static functions signature by including
// that in the overall signature.
// TODO: We *might* need to add events for internal function changes over time,
// but only if they are not pure. We currently only use simple, pure functions.
type TemplateFunc struct {
	// Type is the type of the input vars (2nd) arg if one is specified. Nil
	// is the special undetermined value that is used before type is known.
	Type *types.Type // type of vars

	built bool // was this function built yet?

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *TemplateFunc) String() string {
	return TemplateFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *TemplateFunc) ArgGen(index int) (string, error) {
	seq := []string{templateArgNameTemplate, templateArgNameVars}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *TemplateFunc) sig() *types.Type {
	if obj.Type != nil {
		typ := fmt.Sprintf("func(%s str, %s %s) str", templateArgNameTemplate, templateArgNameVars, obj.Type.String())
		return types.NewType(typ)
	}

	typ := fmt.Sprintf("func(%s str) str", templateArgNameTemplate)
	return types.NewType(typ)
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *TemplateFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(format str) str
	// OR
	// func(format str, arg ?1) str

	if l := len(partialValues); l < 1 || l > 2 {
		return nil, nil, fmt.Errorf("must have at either one or two args")
	}

	var typ *types.Type
	if len(partialValues) == 1 {
		typ = types.NewType(fmt.Sprintf("func(%s str) str", templateArgNameTemplate))
	}

	if len(partialValues) == 2 {
		typ = types.NewType(fmt.Sprintf("func(%s str, %s ?1) str", templateArgNameTemplate, templateArgNameVars))
	}

	return typ, []*interfaces.UnificationInvariant{}, nil
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. It extracts the type of the vars argument,
// which is the dynamic part which can change. That type is used to build our
// function statically.
func (obj *TemplateFunc) Build(typ *types.Type) (*types.Type, error) {
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) != 2 && len(typ.Ord) != 1 {
		return nil, fmt.Errorf("the template function needs exactly one or two args")
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
		return nil, fmt.Errorf("first arg for template must be an str")
	}

	if len(typ.Ord) == 1 { // no args being passed in (boring template)
		obj.built = true
		return obj.sig(), nil
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}
	obj.Type = t1 // extracted vars type is now known!

	obj.built = true
	return obj.sig(), nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *TemplateFunc) Validate() error {
	if !obj.built {
		return fmt.Errorf("function wasn't built yet")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *TemplateFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	var sig *types.Type
	if obj.built {
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: false, // contents of a template might not be pure
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *TemplateFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// run runs a template and returns the result.
func (obj *TemplateFunc) run(ctx context.Context, templateText string, vars types.Value) (string, error) {
	// see: https://golang.org/pkg/text/template/#FuncMap for more info
	// note: we can override any other functions by adding them here...
	funcMap := map[string]interface{}{
		//"test1": func(in interface{}) (interface{}, error) { // ok
		//	return fmt.Sprintf("got(%T): %+v", in, in), nil
		//},
		//"test2": func(in interface{}) interface{} { // NOT ok
		//	panic("panic") // a panic here brings down everything!
		//},
		//"test3": func(foo int64) (string, error) { // ok, but errors
		//	return "", fmt.Errorf("i am an error")
		//},
		//"test4": func(in1, in2 reflect.Value) (reflect.Value, error) { // ok
		//	s := fmt.Sprintf("got: %+v and: %+v", in1, in2)
		//	return reflect.ValueOf(s), nil
		//},
	}

	// FIXME: should we do this once in init() instead, or in the Register
	// function in the simple package?
	// TODO: loop through this map in a sorted, deterministic order
	// XXX: should this use the scope instead (so imports are used properly) ?
	for name, scaffold := range simple.RegisteredFuncs {
		if scaffold.T == nil || scaffold.T.HasUni() {
			if obj.init.Debug {
				obj.init.Logf("warning, function named: `%s` is not unified", name)
			}
			continue
		}
		name = safename(name) // TODO: rename since we can't include dot
		if _, exists := funcMap[name]; exists {
			obj.init.Logf("warning, existing function named: `%s` exists", name)
			continue
		}

		// When template execution invokes a function with an argument
		// list, that list must be assignable to the function's
		// parameter types. Functions meant to apply to arguments of
		// arbitrary type can use parameters of type interface{} or of
		// type reflect.Value.
		f, err := wrap(ctx, name, scaffold) // wrap it so that it meets API expectations
		if err != nil {
			if obj.init.Debug {
				obj.init.Logf("warning, skipping function named: `%s`, err: %v", name, err)
			}
			continue
		}
		funcMap[name] = f // add it
	}

	var err error
	tmpl := template.New(TemplateName)
	tmpl = tmpl.Funcs(funcMap)
	tmpl, err = tmpl.Parse(templateText)
	if err != nil {
		return "", errwrap.Wrapf(err, "template: parse error")
	}

	buf := new(bytes.Buffer)

	if vars == nil {
		// run the template
		if err := tmpl.Execute(buf, nil); err != nil {
			return "", errwrap.Wrapf(err, "template: execution error")
		}
		return buf.String(), nil
	}

	// NOTE: any objects in here can have their methods called by the template!
	var data interface{} // can be many types, eg a struct!
	v := vars.Copy()     // make a copy since we make modifications to it...
Loop:
	// TODO: simplify with Type.Underlying()
	for {
		switch x := v.Type().Kind; x {
		case types.KindBool:
			fallthrough
		case types.KindStr:
			fallthrough
		case types.KindInt:
			fallthrough
		case types.KindFloat:
			// standalone values can be used in templates with a dot
			data = v.Value()
			break Loop

		case types.KindList:
			// TODO: can we improve on this to expose indexes?
			data = v.Value()
			break Loop

		case types.KindMap:
			if v.Type().Key.Cmp(types.TypeStr) != nil {
				return "", errwrap.Wrapf(err, "template: map keys must be str")
			}
			m := make(map[string]interface{})
			for k, v := range v.Map() { // map[Value]Value
				m[k.Str()] = v.Value()
			}
			data = m
			break Loop

		case types.KindStruct:
			m := make(map[string]interface{})
			for k, v := range v.Struct() { // map[string]Value
				m[k] = v.Value()
			}
			data = m
			break Loop

		// TODO: should we allow functions here?
		//case types.KindFunc:

		case types.KindVariant:
			v = v.(*types.VariantValue).V // un-nest and recurse
			continue Loop

		default:
			return "", fmt.Errorf("can't use `%+v` as vars input", x)
		}
	}

	// run the template
	if err := tmpl.Execute(buf, data); err != nil {
		return "", errwrap.Wrapf(err, "template: execution error")
	}
	return buf.String(), nil
}

// Stream returns the changing values that this func has over time.
func (obj *TemplateFunc) Stream(ctx context.Context) error {
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

// Copy is implemented so that the obj.built value is not lost if we copy this
// function.
func (obj *TemplateFunc) Copy() interfaces.Func {
	return &TemplateFunc{
		Type:  obj.Type, // don't copy because we use this after unification
		built: obj.built,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *TemplateFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	tmpl := args[0].Str()

	var vars types.Value // nil
	if len(args) == 2 {
		vars = args[1]
	}

	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}
	result, err := obj.run(ctx, tmpl, vars)
	if err != nil {
		return nil, err // no errwrap needed b/c helper func
	}
	return &types.StrValue{
		V: result,
	}, nil
}

// safename renames the functions so they're valid inside the template. This is
// a limitation of the template library, and it might be worth moving to a new
// one.
func safename(name string) string {
	// TODO: should we pick a different replacement char?
	char := funcs.ReplaceChar // can't be any of: .-#
	result := strings.Replace(name, funcs.ModuleSep, char, -1)
	result = strings.Replace(result, "/", char, -1) // nested imports
	if result == name {
		// No change, so add a prefix for package-less functions... This
		// prevents conflicts from sys.func1 -> sys_func1 which would be
		// a conflict with a top-level function named sys_func1 which is
		// now renamed to _sys_func1.
		return char + name
	}
	return result
}

// wrap builds a function in the format expected by the template engine, and
// returns it as an interface{}. It does so by wrapping our type system and
// function API with what is expected from the reflection API. It returns a
// version that includes the optional second error return value so that our
// functions can return errors without causing a panic.
func wrap(ctx context.Context, name string, scaffold *simple.Scaffold) (_ interface{}, reterr error) {
	defer func() {
		// catch unhandled panics
		if r := recover(); r != nil {
			reterr = fmt.Errorf("panic in template wrap of `%s` function: %+v", name, r)
		}
	}()

	if scaffold.T == nil {
		panic("malformed type")
	}
	if scaffold.T.HasUni() {
		panic("type not unified")
	}
	if scaffold.T.Map == nil {
		panic("malformed func type")
	}
	if len(scaffold.T.Map) != len(scaffold.T.Ord) {
		panic("malformed func length")
	}
	in := []reflect.Type{}
	for _, k := range scaffold.T.Ord {
		t, ok := scaffold.T.Map[k]
		if !ok {
			panic("malformed func order")
		}
		if t == nil {
			panic("malformed func arg")
		}

		in = append(in, t.Reflect())
	}
	ret := scaffold.T.Out.Reflect() // this can panic!
	out := []reflect.Type{ret, errorType}
	var variadic = false // currently not supported in our function value
	typ := reflect.FuncOf(in, out, variadic)

	// wrap our function with the translation that is necessary
	f := func(args []reflect.Value) (results []reflect.Value) { // build
		innerArgs := []types.Value{}
		zeroValue := reflect.Zero(scaffold.T.Out.Reflect()) // zero value of return type
		for _, x := range args {
			v, err := types.ValueOf(x) // reflect.Value -> Value
			if err != nil {
				r := reflect.ValueOf(errwrap.Wrapf(err, "function `%s` errored", name))
				if !r.Type().ConvertibleTo(errorType) { // for fun!
					r = reflect.ValueOf(fmt.Errorf("function `%s` errored: %+v", name, err))
				}
				e := r.Convert(errorType) // must be seen as an `error`
				return []reflect.Value{zeroValue, e}
			}
			innerArgs = append(innerArgs, v)
		}

		result, err := scaffold.F(ctx, innerArgs) // call it
		if err != nil {                           // function errored :(
			// errwrap is a better way to report errors, if allowed!
			r := reflect.ValueOf(errwrap.Wrapf(err, "function `%s` errored", name))
			if !r.Type().ConvertibleTo(errorType) { // for fun!
				r = reflect.ValueOf(fmt.Errorf("function `%s` errored: %+v", name, err))
			}
			e := r.Convert(errorType) // must be seen as an `error`
			return []reflect.Value{zeroValue, e}
		} else if result == nil { // someone wrote a bad function
			r := reflect.ValueOf(fmt.Errorf("function `%s` returned nil", name))
			e := r.Convert(errorType) // must be seen as an `error`
			return []reflect.Value{zeroValue, e}
		}

		nilError := reflect.Zero(errorType)
		return []reflect.Value{reflect.ValueOf(result.Value()), nilError}
	}
	val := reflect.MakeFunc(typ, f)
	return val.Interface(), nil
}

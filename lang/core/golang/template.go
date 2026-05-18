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

	// interfaceType represents a reflection type of interface{} which we
	// use when the underlying mgmt type isn't reflectable. (See the
	// reflectable function for more information.)
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
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
	interfaces.Textarea

	// Type is the type of the input vars (2nd) arg if one is specified. Nil
	// is the special undetermined value that is used before type is known.
	Type *types.Type // type of vars

	built bool // was this function built yet?

	init *interfaces.Init
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
		f, err := obj.wrap(ctx, name, scaffold) // wrap it so that it meets API expectations
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

	v := vars.Copy() // make a copy since we make modifications to it...
	data, err := obj.convert(v)
	if err != nil {
		return "", err
	}

	// run the template
	if err := tmpl.Execute(buf, data); err != nil {
		return "", errwrap.Wrapf(err, "template: execution error")
	}
	return buf.String(), nil
}

// convert helper function.
func (obj *TemplateFunc) convert(v types.Value) (interface{}, error) {
	// TODO: simplify with Type.Underlying()
	switch x := v.Type().Kind; x {
	case types.KindBool:
		fallthrough
	case types.KindStr:
		fallthrough
	case types.KindInt:
		fallthrough
	case types.KindFloat:
		// standalone values can be used in templates with a dot
		return v.Value(), nil

	case types.KindList:
		// If the element type reflects cleanly, return a concrete slice
		// so it can still be passed to typed template functions.
		if reflectable(v.Type()) {
			// TODO: can we improve on this to expose indexes?
			return v.Value(), nil
		}
		// Otherwise (eg: a list of structs with lowercase fields) we
		// recurse so the elements become map[string]interface{}.
		l := []interface{}{}
		for _, x := range v.List() {
			val, err := obj.convert(x)
			if err != nil {
				return nil, err
			}
			l = append(l, val)
		}
		return l, nil

	case types.KindMap:
		// The common case of str keys produces a map[string]interface{}
		// so that template field access (eg: .key) keeps working even
		// when the keys aren't valid (exported) golang identifiers.
		if v.Type().Key.Cmp(types.TypeStr) == nil { // key type is str
			m := make(map[string]interface{})
			for k, v := range v.Map() { // map[Value]Value
				val, err := obj.convert(v)
				if err != nil {
					return nil, err
				}
				m[k.Str()] = val
			}
			return m, nil
		}

		// Otherwise build a real golang map so we can use comparable,
		// non-str keys (eg: structs) and range over them in templates.
		var m reflect.Value         // map[?]interface{}
		for k, v := range v.Map() { // map[Value]Value
			key, err := convertKey(k)
			if err != nil {
				return nil, err
			}
			val, err := obj.convert(v)
			if err != nil {
				return nil, err
			}
			rk := reflect.ValueOf(key)
			if !m.IsValid() { // first iteration
				m = reflect.MakeMap(reflect.MapOf(rk.Type(), interfaceType))
			}
			m.SetMapIndex(rk, reflect.ValueOf(val))
		}
		if !m.IsValid() { // empty map
			return map[string]interface{}{}, nil
		}
		return m.Interface(), nil

	case types.KindStruct:
		m := make(map[string]interface{})
		for k, v := range v.Struct() { // map[string]Value
			val, err := obj.convert(v)
			if err != nil {
				return nil, err
			}
			m[k] = val
		}
		return m, nil

	// TODO: should we allow functions here?
	//case types.KindFunc:

	case types.KindVariant:
		return obj.convert(v.(*types.VariantValue).V) // un-nest and recurse

	default:
		return nil, fmt.Errorf("can't use `%+v` as vars input", x)
	}
}

// wrap builds a function in the format expected by the template engine, and
// returns it as an interface{}. It does so by wrapping our type system and
// function API with what is expected from the reflection API. It returns a
// version that includes the optional second error return value so that our
// functions can return errors without causing a panic.
func (obj *TemplateFunc) wrap(ctx context.Context, name string, scaffold *simple.Scaffold) (_ interface{}, reterr error) {
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
	// If the return type isn't reflectable (eg: a struct with lowercase
	// fields) we hand back an interface{} holding a map[string]interface{}
	// instead, so that lowercase field access keeps working in templates.
	canReflect := reflectable(scaffold.T.Out)
	ret := interfaceType
	if canReflect {
		ret = scaffold.T.Out.Reflect()
	}
	out := []reflect.Type{ret, errorType}
	var variadic = false // currently not supported in our function value
	typ := reflect.FuncOf(in, out, variadic)

	// wrap our function with the translation that is necessary
	f := func(args []reflect.Value) (results []reflect.Value) { // build
		innerArgs := []types.Value{}
		zeroValue := reflect.Zero(ret) // zero value of return type
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
		if canReflect {
			return []reflect.Value{reflect.ValueOf(result.Value()), nilError}
		}

		// non-reflectable return: convert to map[string]interface{} etc.
		val, err := obj.convert(result)
		if err != nil {
			r := reflect.ValueOf(errwrap.Wrapf(err, "function `%s` errored", name))
			if !r.Type().ConvertibleTo(errorType) { // for fun!
				r = reflect.ValueOf(fmt.Errorf("function `%s` errored: %+v", name, err))
			}
			e := r.Convert(errorType) // must be seen as an `error`
			return []reflect.Value{zeroValue, e}
		}
		iv := reflect.New(interfaceType).Elem()
		iv.Set(reflect.ValueOf(val))
		return []reflect.Value{iv, nilError}
	}
	val := reflect.MakeFunc(typ, f)
	return val.Interface(), nil
}

// Copy is implemented so that the obj.built value is not lost if we copy this
// function.
func (obj *TemplateFunc) Copy() interfaces.Func {
	return &TemplateFunc{
		Textarea: obj.Textarea,

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

// reflectable returns true if the type can be passed through the golang reflect
// API without panicking. It is false for any struct that contains an unexported
// (lowercase) field name, since reflect.StructOf panics on those. In that case
// we represent the value as a map[string]interface{} instead so that lowercase
// field access keeps working inside templates.
func reflectable(typ *types.Type) bool {
	if typ == nil {
		return false
	}
	switch typ.Kind {
	case types.KindBool, types.KindStr, types.KindInt, types.KindFloat:
		return true

	case types.KindList:
		return reflectable(typ.Val)

	case types.KindMap:
		return reflectable(typ.Key) && reflectable(typ.Val)

	case types.KindStruct:
		for _, k := range typ.Ord {
			if strings.Title(k) != k { // unexported field
				return false
			}
			if !reflectable(typ.Map[k]) {
				return false
			}
		}
		return true

	case types.KindVariant:
		return reflectable(typ.Var)
	}

	return false // something else (eg: func)
}

// convertKey is like convert, except it produces a comparable golang value that
// is suitable for use as a map key inside a template. Maps aren't comparable in
// golang, so structs are turned into real golang structs (with exported, titled
// field names) instead of the map[string]interface{} that convert would
// otherwise build.
func convertKey(v types.Value) (interface{}, error) {
	switch x := v.Type().Kind; x {
	case types.KindBool:
		fallthrough
	case types.KindStr:
		fallthrough
	case types.KindInt:
		fallthrough
	case types.KindFloat:
		return v.Value(), nil

	case types.KindStruct:
		fields := []reflect.StructField{}
		vals := []reflect.Value{}
		for _, k := range v.Type().Ord { // deterministic field order
			val, err := convertKey(v.Struct()[k])
			if err != nil {
				return nil, err
			}
			rv := reflect.ValueOf(val)
			fields = append(fields, reflect.StructField{
				Name: strings.Title(k), // must be exported
				Type: rv.Type(),
			})
			vals = append(vals, rv)
		}
		st := reflect.New(reflect.StructOf(fields)).Elem()
		for i, rv := range vals {
			st.Field(i).Set(rv)
		}
		return st.Interface(), nil

	case types.KindVariant:
		return convertKey(v.(*types.VariantValue).V) // un-nest

	default:
		return nil, fmt.Errorf("can't use `%+v` as a template map key", x)
	}
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

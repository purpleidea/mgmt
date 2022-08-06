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

package core // TODO: should this be in its own individual package?

import (
	"bytes"
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

var (
	// errorType represents a reflection type of error as seen in:
	// https://github.com/golang/go/blob/ec62ee7f6d3839fe69aeae538dadc1c9dc3bf020/src/text/template/exec.go#L612
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

func init() {
	funcs.Register("template", func() interfaces.Func { return &TemplateFunc{} })
}

const (
	// TemplateName is the name of our template as required by the template
	// library.
	TemplateName = "template"

	argNameTemplate = "template"
	argNameVars     = "vars"
)

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
	// NoVars is set to true instead of specifying Type if we have a boring
	// template that takes no args.
	NoVars bool

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result *string // last calculated output

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *TemplateFunc) ArgGen(index int) (string, error) {
	seq := []string{argNameTemplate, argNameVars}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *TemplateFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(format string) string
	// OR
	// func(format string, arg variant) string

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
			if l := len(cfavInvar.Args); l > 2 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}
			// we can either have one arg or two

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

			// TODO: if the template is known statically, we could
			// parse it to check for variable safety if we wanted!
			//value, err := cfavInvar.Args[0].Value() // is it known?
			//if err != nil {
			//}

			// full function
			mapped := make(map[string]interfaces.Expr)
			ordered := []string{formatName}
			mapped[formatName] = dummyFormat

			if len(cfavInvar.Args) == 2 { // two args is more complex
				argName, err := obj.ArgGen(1) // 1st arg after 0
				if err != nil {
					return nil, err
				}
				if argName == argNameTemplate {
					return nil, fmt.Errorf("could not build function with %d args", 1)
				}

				dummyArg := &interfaces.ExprAny{}

				// speculate about the type? (maybe redundant)
				if typ, err := cfavInvar.Args[1].Type(); err == nil {
					invar := &interfaces.EqualsInvariant{
						Expr: dummyArg,
						Type: typ,
					}
					invariants = append(invariants, invar)
				}

				if typ, exists := solved[cfavInvar.Args[1]]; exists { // alternate way to lookup type
					invar := &interfaces.EqualsInvariant{
						Expr: dummyArg,
						Type: typ,
					}
					invariants = append(invariants, invar)
				}

				// expression must match type of the input arg
				invar := &interfaces.EqualityInvariant{
					Expr1: dummyArg,
					Expr2: cfavInvar.Args[1],
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

// Polymorphisms returns the possible type signatures for this template. In this
// case, since the second argument can be an infinite number of values, it
// instead returns either the final precise type (if it can be gleamed from the
// input partials) or if it cannot, it returns a single entry with the complete
// type but with the variable second argument specified as a `variant` type. If
// it encounters any partial type specifications which are not possible, then it
// errors out. This could happen if you specified a non string template arg.
// XXX: is there a better API than returning a buried `variant` type?
func (obj *TemplateFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as second arg for now -- maybe there's a better way?
	str := fmt.Sprintf("func(%s str, %s variant) str", argNameTemplate, argNameVars)
	variant := []*types.Type{types.NewType(str)}

	if partialType == nil {
		return variant, nil
	}

	if partialType.Out != nil && partialType.Out.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("return value of template must be str")
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 2 && len(ord) != 1 {
			return nil, fmt.Errorf("must have exactly one or two args in template func")
		}
		if t, exists := partialType.Map[ord[0]]; exists && t != nil {
			if t.Cmp(types.TypeStr) != nil {
				return nil, fmt.Errorf("first arg for template must be an str")
			}
		}
		if len(ord) == 1 { // no args being passed in (boring template)
			return []*types.Type{types.NewType(fmt.Sprintf("func(%s str) str", argNameTemplate))}, nil

		} else if t, exists := partialType.Map[ord[1]]; exists && t != nil {
			// known vars type! w00t!
			return []*types.Type{types.NewType(fmt.Sprintf("func(%s str, %s %s) str", argNameTemplate, argNameVars, t.String()))}, nil
		}
	}

	return variant, nil
}

// Build takes the now known function signature and stores it so that this
// function can appear to be static. It extracts the type of the vars argument,
// which is the dynamic part which can change. That type is used to build our
// function statically.
func (obj *TemplateFunc) Build(typ *types.Type) error {
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}
	if len(typ.Ord) != 2 && len(typ.Ord) != 1 {
		return fmt.Errorf("the template function needs exactly one or two args")
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
		return fmt.Errorf("first arg for template must be an str")
	}

	if len(typ.Ord) == 1 { // no args being passed in (boring template)
		obj.NoVars = true
		return nil
	}

	t1, exists := typ.Map[typ.Ord[1]]
	if !exists || t1 == nil {
		return fmt.Errorf("second arg must be specified")
	}
	obj.Type = t1 // extracted vars type is now known!

	return nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *TemplateFunc) Validate() error {
	if obj.Type == nil && !obj.NoVars { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *TemplateFunc) Info() *interfaces.Info {
	var sig *types.Type
	if obj.NoVars {
		str := fmt.Sprintf("func(%s str) str", argNameTemplate)
		sig = types.NewType(str)

	} else if obj.Type != nil { // don't panic if called speculatively
		str := fmt.Sprintf("func(%s str, %s %s) str", argNameTemplate, argNameVars, obj.Type.String())
		sig = types.NewType(str)
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *TemplateFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// run runs a template and returns the result.
func (obj *TemplateFunc) run(templateText string, vars types.Value) (string, error) {
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
	for name, fn := range simple.RegisteredFuncs {
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
		f := wrap(name, fn) // wrap it so that it meets API expectations
		funcMap[name] = f   // add it
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
func (obj *TemplateFunc) Stream() error {
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

			st := input.Struct()

			tmpl := st[argNameTemplate].Str()
			vars, exists := st[argNameVars]
			if !exists {
				vars = nil
			}

			result, err := obj.run(tmpl, vars)
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
func (obj *TemplateFunc) Close() error {
	close(obj.closeChan)
	return nil
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
func wrap(name string, fn *types.FuncValue) interface{} {
	if fn.T.Map == nil {
		panic("malformed func type")
	}
	if len(fn.T.Map) != len(fn.T.Ord) {
		panic("malformed func length")
	}
	in := []reflect.Type{}
	for _, k := range fn.T.Ord {
		t, ok := fn.T.Map[k]
		if !ok {
			panic("malformed func order")
		}
		if t == nil {
			panic("malformed func arg")
		}

		in = append(in, t.Reflect())
	}
	out := []reflect.Type{fn.T.Out.Reflect(), errorType}
	var variadic = false // currently not supported in our function value
	typ := reflect.FuncOf(in, out, variadic)

	// wrap our function with the translation that is necessary
	f := func(args []reflect.Value) (results []reflect.Value) { // build
		innerArgs := []types.Value{}
		zeroValue := reflect.Zero(fn.T.Out.Reflect()) // zero value of return type
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

		result, err := fn.Call(innerArgs) // call it
		if err != nil {                   // function errored :(
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
	return val.Interface()
}

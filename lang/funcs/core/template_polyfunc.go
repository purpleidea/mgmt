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

package core // TODO: should this be in its own individual package?

import (
	"bytes"
	"fmt"
	"text/template"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"

	errwrap "github.com/pkg/errors"
)

func init() {
	funcs.Register("template", func() interfaces.Func { return &TemplateFunc{} })
}

// TemplateName is the name of our template as required by the template library.
const TemplateName = "template"

// TemplateFunc is a static polymorphic function that compiles a template and
// returns the output as a string. It bases its output on the values passed in
// to it. It examines the type of the second argument (the input data vars) at
// compile time and then determines the static functions signature by including
// that in the overall signature.
// XXX: do we need to add events if any of the internal functions change over time?
type TemplateFunc struct {
	Type *types.Type // type of vars

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result string // last calculated output

	closeChan chan struct{}
}

// Polymorphisms returns the possible type signatures for this template. In this
// case, since the second argument can be an infinite number of values, it
// instead returns either the final precise type (if it can be gleamed from the
// input partials) or if it cannot, it returns a single entry with the complete
// type but with the variable second argument specified as a `variant` type.
// If it encounters any partial type specifications which are not possible, then
// it errors out. This could happen if you specified a non string template arg.
// XXX: is there a better API than returning a buried `variant` type?
func (obj *TemplateFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: return `variant` as second arg for now -- maybe there's a better way?
	variant := []*types.Type{types.NewType("func(a str, b variant) str")}

	if partialType == nil {
		return variant, nil
	}

	if partialType.Out != nil && partialType.Out.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("return value of template must be str")
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) != 2 {
			return nil, fmt.Errorf("must have exactly two args in template func")
		}
		if t, exists := partialType.Map[ord[0]]; exists && t != nil {
			if t.Cmp(types.TypeStr) != nil {
				return nil, fmt.Errorf("first arg for template must be an str")
			}
		}
		if t, exists := partialType.Map[ord[1]]; exists && t != nil {
			// known vars type! w00t!
			return []*types.Type{types.NewType(fmt.Sprintf("func(a str, b %s) str", t.String()))}, nil
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
	if len(typ.Ord) != 2 {
		return fmt.Errorf("the template function needs exactly two args")
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
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *TemplateFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(template str, vars %s) str", obj.Type.String())),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this fact.
func (obj *TemplateFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// run runs a template and returns the result.
func (obj *TemplateFunc) run(templateText string, vars types.Value) (string, error) {
	funcMap := map[string]interface{}{
		// XXX: can these functions come from normal funcValue things
		// that we build for the interfaces.Func part?
		// TODO: add a bunch of stdlib-like stuff here...
		"datetimeprint": func(epochDelta int64) string { // TODO: rename
			return time.Unix(epochDelta, 0).String()
		},
	}

	var err error
	tmpl := template.New(TemplateName)
	tmpl = tmpl.Funcs(funcMap)
	tmpl, err = tmpl.Parse(templateText)
	if err != nil {
		return "", errwrap.Wrapf(err, "template: parse error")
	}

	buf := new(bytes.Buffer)
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

			tmpl := input.Struct()["template"].Str()
			vars := input.Struct()["vars"]

			result, err := obj.run(tmpl, vars)
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

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *TemplateFunc) Close() error {
	close(obj.closeChan)
	return nil
}

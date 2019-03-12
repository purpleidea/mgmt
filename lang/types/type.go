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

package types

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// Basic types defined here as a convenience for use with Type.Cmp(X).
var (
	TypeBool    = NewType("bool")
	TypeStr     = NewType("str")
	TypeInt     = NewType("int")
	TypeFloat   = NewType("float")
	TypeVariant = NewType("variant")
)

//go:generate stringer -type=Kind -output=kind_stringer.go

// The Kind represents the base type of each value.
type Kind int // this used to be called Type

// Each Kind represents a type in the language type system.
const (
	KindNil Kind = iota
	KindBool
	KindStr
	KindInt
	KindFloat
	KindList
	KindMap
	KindStruct
	KindFunc
	KindVariant
)

// Type is the datastructure representing any type. It can be recursive for
// container types like lists, maps, and structs.
// TODO: should we create a `Type` interface?
type Type struct {
	Kind Kind

	Val *Type            // if Kind == List, use Val only
	Key *Type            // if Kind == Map, use Val and Key
	Map map[string]*Type // if Kind == Struct, use Map and Ord (for order)
	Ord []string
	Out *Type // if Kind == Func, use Map and Ord for Input, Out for Output
	Var *Type // if Kind == Variant, use Var only
}

// TypeOf takes a reflect.Type and returns an equivalent *Type. It removes any
// pointers since our language does not support pointers. It returns nil if it
// cannot represent the type in our type system. Common examples of things it
// cannot express include reflect.Invalid, reflect.Interface, Reflect.Complex128
// and more. It is not reversible because some information may be either added
// or lost. For example, reflect.Array and reflect.Slice are both converted to
// a Type of KindList, and KindFunc names the arguments of a func sequentially.
// The lossy inverse of this is Reflect.
func TypeOf(t reflect.Type) (*Type, error) {
	typ := t
	kind := typ.Kind()
	for kind == reflect.Ptr {
		typ = typ.Elem() // un-nest one pointer
		kind = typ.Kind()
	}

	switch kind { // match on destination field kind
	case reflect.Bool:
		return &Type{
			Kind: KindBool,
		}, nil

	case reflect.String:
		return &Type{
			Kind: KindStr,
		}, nil

	case reflect.Int, reflect.Int64, reflect.Int32, reflect.Int16, reflect.Int8:
		fallthrough
	case reflect.Uint, reflect.Uint64, reflect.Uint32, reflect.Uint16, reflect.Uint8:
		// we have only one kind of int type
		return &Type{
			Kind: KindInt,
		}, nil

	case reflect.Float64, reflect.Float32:
		return &Type{
			Kind: KindFloat,
		}, nil

	case reflect.Array, reflect.Slice:
		val, err := TypeOf(typ.Elem())
		if err != nil {
			return nil, err
		}

		return &Type{
			Kind: KindList,
			Val:  val,
		}, nil

	case reflect.Map:
		key, err := TypeOf(typ.Key()) // Key returns a map type's key type.
		if err != nil {
			return nil, err
		}
		val, err := TypeOf(typ.Elem()) // Elem returns a type's element type.
		if err != nil {
			return nil, err
		}

		return &Type{
			Kind: KindMap,
			Key:  key,
			Val:  val,
		}, nil

	case reflect.Struct:
		m := make(map[string]*Type)
		ord := []string{}

		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			tt, err := TypeOf(field.Type)
			if err != nil {
				return nil, err
			}
			// TODO: should we skip over fields with field.Anonymous ?
			m[field.Name] = tt
			ord = append(ord, field.Name) // in order
			// NOTE: we discard the field.Tag data
		}

		return &Type{
			Kind: KindStruct,
			Map:  m,
			Ord:  ord,
		}, nil

	case reflect.Func:
		m := make(map[string]*Type)
		ord := []string{}

		for i := 0; i < typ.NumIn(); i++ {
			tt, err := TypeOf(typ.In(i))
			if err != nil {
				return nil, err
			}
			name := fmt.Sprintf("%d", i) // invent a function arg name
			m[name] = tt
			ord = append(ord, name) // in order
		}

		var out *Type
		var err error
		// we currently leave out nil if there are no return values
		if c := typ.NumOut(); c == 1 {
			out, err = TypeOf(typ.Out(0))
			if err != nil {
				return nil, err
			}
		} else if c > 1 {
			// if we have multiple return values, we could return a
			// struct, but for now let's just complain...
			return nil, fmt.Errorf("func has %d return values", c)
		}
		// nothing special to do if type is variadic, it's a list...
		//if typ.IsVariadic() {
		//}

		return &Type{
			Kind: KindFunc,
			Map:  m,
			Ord:  ord,
			Out:  out,
		}, nil

	// TODO: should this return a variant type?
	//case reflect.Interface:

	default:
		return nil, fmt.Errorf("unable to represent type of %s", typ.String())
	}
}

// NewType creates the Type from the string representation.
func NewType(s string) *Type {
	switch s {
	case "bool":
		return &Type{
			Kind: KindBool,
		}
	case "str":
		return &Type{
			Kind: KindStr,
		}
	case "int":
		return &Type{
			Kind: KindInt,
		}
	case "float":
		return &Type{
			Kind: KindFloat,
		}
	}

	// KindList
	if strings.HasPrefix(s, "[]") {
		val := NewType(s[len("[]"):])
		if val == nil {
			return nil
		}
		return &Type{
			Kind: KindList,
			Val:  val,
		}
	}

	// KindMap
	if strings.HasPrefix(s, "map{") && strings.HasSuffix(s, "}") {
		s := s[len("map{") : len(s)-1]
		if s == "" { // it is empty
			return nil
		}
		// {<type>: <type>} // map
		var found int
		var delta int
		for i, c := range s {
			if c == '{' { // open
				delta++
			}
			if c == '}' { // close
				delta--
			}
			if c == ':' && delta == 0 {
				found = i
			}
		}
		if found == 0 || delta != 0 { // nope if we fall off the end...
			return nil
		}

		key := NewType(strings.Trim(s[:found], " "))
		if key == nil {
			return nil
		}
		val := NewType(strings.Trim(s[found+1:], " "))
		if val == nil {
			return nil
		}
		return &Type{
			Kind: KindMap,
			Key:  key,
			Val:  val,
		}
	}

	// KindStruct
	if strings.HasPrefix(s, "struct{") && strings.HasSuffix(s, "}") {
		s := s[len("struct{") : len(s)-1]
		keys := []string{}
		tmap := make(map[string]*Type)

		for { // while we still have struct pairs to process...
			s = strings.Trim(s, " ")
			if s == "" {
				break // done
			}

			sep := strings.Index(s, " ")
			if sep <= 0 {
				return nil
			}
			key := s[:sep] // FIXME: check there are no special chars in key
			keys = append(keys, key)

			s = s[sep+1:] // what's next

			var found int
			var delta int
			for i, c := range s {
				if c == '{' { // open
					delta++
				}
				if c == '}' { // close
					delta--
				}
				if c == ';' && delta == 0 { // is there nesting?
					found = i
					break // stop at first semicolon
				}
			}
			if delta != 0 { // nope if we're still nested...
				return nil
			}
			if found == 0 { // no semicolon
				found = len(s) - 1 // last char
			}

			var trim int
			if s[found:found+1] == ";" {
				trim = 1
			}

			typ := NewType(strings.Trim(s[:found+1-trim], " "))
			if typ == nil {
				return nil
			}
			tmap[key] = typ // add type
			s = s[found+1:] // what's left?
		}

		return &Type{
			Kind: KindStruct,
			Ord:  keys,
			Map:  tmap,
		}
	}

	// KindFunc
	if strings.HasPrefix(s, "func(") {
		// find end of function...
		var found int
		var delta = 1 // we've got the first open bracket
		for i := len("func("); i < len(s); i++ {
			c := s[i]
			if c == '(' { // open
				delta++
			}
			if c == ')' { // close
				delta--
			}
			if delta == 0 {
				found = i
				break
			}
		}
		if delta != 0 { // nesting is not paired...
			return nil
		}

		out := strings.Trim(s[found+1:], " ") // return type
		s := s[len("func("):found]            // contents of function
		keys := []string{}
		tmap := make(map[string]*Type)

		for { // while we still have function arguments to process...
			s = strings.Trim(s, " ")
			if s == "" {
				break // done
			}
			var key string

			// arg naming code, which allows for optional arg names
			for i, c := range s { // looking for an arg name
				if c == ',' { // there was no arg name
					break
				}
				if c == '{' || c == '(' { // not an arg name
					break
				}
				if c == '}' || c == ')' { // unexpected format?
					return nil
				}
				if c == ' ' { // done
					key = s[:i] // found a key?
					s = s[i+1:] // what's next
					break
				}
			}

			// just name the keys 0, 1, 2, N...
			if key == "" {
				key = fmt.Sprintf("%d", len(keys))
			}
			keys = append(keys, key)

			var found int
			var delta int
			for i, c := range s {
				if c == '(' { // open
					delta++
				}
				if c == ')' { // close
					delta--
				}
				if c == ',' && delta == 0 { // is there nesting?
					found = i
					break // stop at first comma
				}
			}
			if delta != 0 { // nope if we're still nested...
				return nil
			}
			if found == 0 { // no comma
				found = len(s) - 1 // last char
			}

			var trim int
			if s[found:found+1] == "," {
				trim = 1
			}

			typ := NewType(strings.Trim(s[:found+1-trim], " "))
			if typ == nil {
				return nil
			}
			tmap[key] = typ // add type
			s = s[found+1:] // what's left?
		}

		// return type
		var tail *Type
		if out != "" { // allow functions with no return type (in parser)
			tail = NewType(out)
			if tail == nil {
				return nil
			}
		}

		return &Type{
			Kind: KindFunc,
			Ord:  keys,
			Map:  tmap,
			Out:  tail,
		}
	}

	// KindVariant
	if s == "variant" {
		return &Type{
			Kind: KindVariant,
		}
	}

	return nil // error (this also matches the empty string as input)
}

// New creates a new Value of this type.
func (obj *Type) New() Value {
	switch obj.Kind {
	case KindBool:
		return NewBool()
	case KindStr:
		return NewStr()
	case KindInt:
		return NewInt()
	case KindFloat:
		return NewFloat()
	case KindList:
		return NewList(obj)
	case KindMap:
		return NewMap(obj)
	case KindStruct:
		return NewStruct(obj)
	case KindFunc:
		return NewFunc(obj)
	case KindVariant:
		return NewVariant(obj)
	}
	panic("malformed type")
}

// String returns the textual representation for this type.
func (obj *Type) String() string {
	switch obj.Kind {
	case KindBool:
		return "bool"
	case KindStr:
		return "str"
	case KindInt:
		return "int"
	case KindFloat:
		return "float"

	case KindList:
		if obj.Val == nil {
			panic("malformed list type")
		}
		return "[]" + obj.Val.String()

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}
		return fmt.Sprintf("map{%s: %s}", obj.Key.String(), obj.Val.String())

	case KindStruct: // {a bool; b int}
		if obj.Map == nil {
			panic("malformed struct type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed struct length")
		}
		var s = make([]string, len(obj.Ord))
		for i, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			s[i] = fmt.Sprintf("%s %s", k, t.String())
		}
		return fmt.Sprintf("struct{%s}", strings.Join(s, "; "))

	case KindFunc:
		if obj.Map == nil {
			panic("malformed func type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed func length")
		}
		var s = make([]string, len(obj.Ord))
		for i, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			//s[i] = fmt.Sprintf("%s %s", k, t.String()) // strict
			s[i] = t.String()
		}
		var out string
		if obj.Out != nil {
			out = fmt.Sprintf(" %s", obj.Out.String())
		}
		return fmt.Sprintf("func(%s)%s", strings.Join(s, ", "), out)

	case KindVariant:
		return "variant"
	}

	panic("malformed type")
}

// Cmp compares this type to another.
func (obj *Type) Cmp(typ *Type) error {
	if obj == nil || typ == nil {
		return fmt.Errorf("cannot compare to nil")
	}

	// TODO: is this correct?
	// recurse into variants if we want base type comparisons
	//if obj.Kind == KindVariant {
	//	return obj.Var.Cmp(t)
	//}
	//if t.Kind == KindVariant {
	//	return obj.Cmp(t.Var)
	//}

	if obj.Kind != typ.Kind {
		return fmt.Errorf("base kind does not match (%+v != %+v)", obj.Kind, typ.Kind)
	}
	switch obj.Kind {
	case KindBool:
		return nil
	case KindStr:
		return nil
	case KindInt:
		return nil
	case KindFloat:
		return nil

	case KindList:
		if obj.Val == nil || typ.Val == nil {
			panic("malformed list type")
		}
		return obj.Val.Cmp(typ.Val)

	case KindMap:
		if obj.Key == nil || obj.Val == nil || typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		kerr := obj.Key.Cmp(typ.Key)
		verr := obj.Val.Cmp(typ.Val)
		if kerr != nil && verr != nil {
			return errwrap.Append(kerr, verr) // two errors
		}
		if kerr != nil {
			return kerr
		}
		if verr != nil {
			return verr
		}
		return nil

	case KindStruct: // {a bool; b int}
		if obj.Map == nil || typ.Map == nil {
			panic("malformed struct type")
		}
		if len(obj.Ord) != len(typ.Ord) {
			return fmt.Errorf("struct field count differs")
		}
		for i, k := range obj.Ord {
			if k != typ.Ord[i] {
				return fmt.Errorf("struct fields differ")
			}
		}
		for _, k := range obj.Ord { // loop map in deterministic order
			t1, ok := obj.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			t2, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t1 == nil || t2 == nil {
				panic("malformed struct field")
			}
			if err := t1.Cmp(t2); err != nil {
				return err
			}
		}
		return nil

	case KindFunc:
		if obj.Map == nil || typ.Map == nil {
			panic("malformed func type")
		}
		if len(obj.Ord) != len(typ.Ord) {
			return fmt.Errorf("func arg count differs")
		}
		// needed for strict cmp only...
		//for i, k := range obj.Ord {
		//	if k != typ.Ord[i] {
		//		return fmt.Errorf("func arg differs")
		//	}
		//}
		//for _, k := range obj.Ord { // loop map in deterministic order
		//	t1, ok := obj.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	t2, ok := typ.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	if t1 == nil || t2 == nil {
		//		panic("malformed func arg")
		//	}
		//	if err := t1.Cmp(t2); err != nil {
		//		return err
		//	}
		//}

		// if we're not comparing arg names, get the two lists of types
		for i := 0; i < len(obj.Ord); i++ {
			t1, ok := obj.Map[obj.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t1 == nil {
				panic("malformed func arg")
			}

			t2, ok := typ.Map[typ.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t2 == nil {
				panic("malformed func arg")
			}

			if err := t1.Cmp(t2); err != nil {
				return err
			}
		}

		if obj.Out != nil || typ.Out != nil {
			if err := obj.Out.Cmp(typ.Out); err != nil {
				return err
			}
		}
		return nil

	// TODO: is this correct?
	case KindVariant:
		if typ.Kind != KindVariant {
			return fmt.Errorf("variant only compares with other variants")
		}
		// TODO: should we Cmp obj.Var with typ.Var ? -- not necessarily
		return nil
	}
	return fmt.Errorf("unknown kind")
}

// Copy copies this type so that inplace modification won't affect the original.
func (obj *Type) Copy() *Type {
	return NewType(obj.String()) // hack to do this easily
}

// Reflect returns a representative type satisfying the golang Type Interface.
// The lossy inverse of this is TypeOf.
func (obj *Type) Reflect() reflect.Type {
	switch obj.Kind {
	case KindBool:
		return reflect.TypeOf(bool(false))
	case KindStr:
		return reflect.TypeOf(string(""))
	case KindInt:
		return reflect.TypeOf(int64(0))
	case KindFloat:
		return reflect.TypeOf(float64(0))

	case KindList:
		if obj.Val == nil {
			panic("malformed list type")
		}
		return reflect.SliceOf(obj.Val.Reflect()) // recurse

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}
		return reflect.MapOf(obj.Key.Reflect(), obj.Val.Reflect()) // dual recurse

	case KindStruct: // {a bool; b int}
		if obj.Map == nil {
			panic("malformed struct type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed struct length")
		}

		fields := []reflect.StructField{}
		for _, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}

			fields = append(fields, reflect.StructField{
				Name: k, // struct field name
				Type: t.Reflect(),
				//Tag:  `mgmt:"foo"`, // unused
			})
		}

		return reflect.StructOf(fields)

	case KindFunc:
		if obj.Map == nil {
			panic("malformed func type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed func length")
		}

		in := []reflect.Type{}
		for _, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func arg")
			}

			in = append(in, t.Reflect())
		}

		out := []reflect.Type{} // only one return arg
		if obj.Out != nil {
			out = append(out, obj.Out.Reflect())
		}
		var variadic = false // we don't support variadic functions atm

		return reflect.FuncOf(in, out, variadic)

	case KindVariant:
		var x interface{}
		return reflect.TypeOf(x) // TODO: is this correct?
	}

	panic("malformed type")
}

// Underlying returns the underlying type of the type in question. For variants,
// this unpacks them recursively, for everything else this returns itself.
func (obj *Type) Underlying() *Type {
	typ := obj // initial exposed type
	for {
		if typ.Kind != KindVariant {
			return typ
		}
		typ = typ.Var // unpack child type of variant
	}
}

// HasVariant tells us if the type contains any mention of the Variant type.
func (obj *Type) HasVariant() bool {
	if obj == nil {
		return false
	}
	switch obj.Kind {
	case KindBool:
		return false
	case KindStr:
		return false
	case KindInt:
		return false
	case KindFloat:
		return false

	case KindList:
		if obj.Val == nil {
			panic("malformed list type")
		}
		return obj.Val.HasVariant()

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}
		return obj.Key.HasVariant() || obj.Val.HasVariant()

	case KindStruct: // {a bool; b int}
		if obj.Map == nil {
			panic("malformed struct type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed struct length")
		}
		for _, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t == nil {
				panic("malformed struct field")
			}
			if t.HasVariant() {
				return true
			}
		}
		return false

	case KindFunc:
		if obj.Map == nil {
			panic("malformed func type")
		}
		if len(obj.Map) != len(obj.Ord) {
			panic("malformed func length")
		}
		for _, k := range obj.Ord {
			t, ok := obj.Map[k]
			if !ok {
				panic("malformed func order")
			}
			if t == nil {
				panic("malformed func field")
			}
			if t.HasVariant() {
				return true
			}
		}
		if obj.Out != nil {
			if obj.Out.HasVariant() {
				return true
			}
		}
		return false

	case KindVariant:
		return true // found it!
	}

	panic("malformed type")
}

// ComplexCmp tells us if the input type is compatible with the concrete one. It
// can match against types containing variants, or against partial types. If the
// two types are equivalent, it will return nil. If the input type is identical,
// and concrete, the return status will be the empty string. If this match finds
// a possibility against a partial type, the status will be set to the "partial"
// string, and if it is compatible with the variant type it will be "variant"...
// Comparing to a partial can only match "impossible" (error) or possible (nil).
func (obj *Type) ComplexCmp(typ *Type) (string, error) {
	// match simple "placeholder" variants... skip variants w/ sub types
	isVariant := func(t *Type) bool { return t.Kind == KindVariant && t.Var == nil }

	if obj == nil {
		return "", fmt.Errorf("can't cmp from a nil type")
	}
	// XXX: can we relax this to allow variants matching against partials?
	if obj.HasVariant() {
		return "", fmt.Errorf("only input can contain variants")
	}

	if typ == nil { // match
		return "partial", nil // compatible :)
	}
	if isVariant(typ) { // match
		return "variant", nil // compatible :)
	}

	if obj.Kind != typ.Kind {
		return "", fmt.Errorf("base kind does not match (%+v != %+v)", obj.Kind, typ.Kind)
	}

	// only container types are left to match...
	switch obj.Kind {
	case KindBool:
		return "", nil // regular cmp
	case KindStr:
		return "", nil
	case KindInt:
		return "", nil
	case KindFloat:
		return "", nil

	case KindList:
		if obj.Val == nil {
			panic("malformed list type")
		}
		if typ.Val == nil {
			return "partial", nil
		}

		return obj.Val.ComplexCmp(typ.Val)

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}

		if typ.Key == nil && typ.Val == nil {
			return "partial", nil
		}
		if typ.Key == nil {
			return obj.Val.ComplexCmp(typ.Val)
		}
		if typ.Val == nil {
			return obj.Key.ComplexCmp(typ.Key)
		}

		kstatus, kerr := obj.Key.ComplexCmp(typ.Key)
		vstatus, verr := obj.Val.ComplexCmp(typ.Val)
		if kerr != nil && verr != nil {
			return "", errwrap.Append(kerr, verr) // two errors
		}
		if kerr != nil {
			return "", kerr
		}
		if verr != nil {
			return "", verr
		}

		if kstatus == "" && vstatus == "" {
			return "", nil
		} else if kstatus != "" && vstatus == "" {
			return kstatus, nil
		} else if vstatus != "" && kstatus == "" {
			return vstatus, nil
		}

		// optimization, redundant
		//if kstatus == vstatus { // both partial or both variant...
		//	return kstatus, nil
		//}

		var isVariant, isPartial bool
		if kstatus == "variant" || vstatus == "variant" {
			isVariant = true
		}
		if kstatus == "partial" || vstatus == "partial" {
			isPartial = true
		}
		if kstatus == "both" || vstatus == "both" {
			isVariant = true
			isPartial = true
		}

		if !isVariant && !isPartial {
			return "", nil
		}
		if isVariant {
			return "variant", nil
		}
		if isPartial {
			return "partial", nil
		}

		//return "", fmt.Errorf("matches as both partial and variant")
		return "both", nil

	case KindStruct: // {a bool; b int}
		if obj.Map == nil {
			panic("malformed struct type")
		}
		if typ.Map == nil {
			return "partial", nil
		}

		if len(obj.Ord) != len(typ.Ord) {
			return "", fmt.Errorf("struct field count differs")
		}
		for i, k := range obj.Ord {
			if k != typ.Ord[i] {
				return "", fmt.Errorf("struct fields differ")
			}
		}
		var isVariant, isPartial bool
		for _, k := range obj.Ord { // loop map in deterministic order
			t1, ok := obj.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			t2, ok := typ.Map[k]
			if !ok {
				panic("malformed struct order")
			}
			if t1 == nil {
				panic("malformed struct field")
			}
			if t2 == nil {
				isPartial = true
				continue
			}

			status, err := t1.ComplexCmp(t2)
			if err != nil {
				return "", err
			}
			if status == "variant" {
				isVariant = true
			}
			if status == "partial" {
				isPartial = true
			}
			if status == "both" {
				isVariant = true
				isPartial = true
			}
		}

		if !isVariant && !isPartial {
			return "", nil
		}
		if isVariant {
			return "variant", nil
		}
		if isPartial {
			return "partial", nil
		}

		//return "", fmt.Errorf("matches as both partial and variant")
		return "both", nil

	case KindFunc:
		if obj.Map == nil {
			panic("malformed func type")
		}
		if typ.Map == nil {
			return "partial", nil
		}

		if len(obj.Ord) != len(typ.Ord) {
			return "", fmt.Errorf("func arg count differs")
		}

		// needed for strict cmp only...
		//for i, k := range obj.Ord {
		//	if k != typ.Ord[i] {
		//		return "", fmt.Errorf("func arg differs")
		//	}
		//}
		//var isVariant, isPartial bool
		//for _, k := range obj.Ord { // loop map in deterministic order
		//	t1, ok := obj.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	t2, ok := typ.Map[k]
		//	if !ok {
		//		panic("malformed func order")
		//	}
		//	if t1 == nil {
		//		panic("malformed func arg")
		//	}
		//	if t2 == nil {
		//		isPartial = true
		//		continue
		//	}
		//
		//	status, err := t1.ComplexCmp(t2)
		//	if err != nil {
		//		return "", err
		//	}
		//	if status == "variant" {
		//		isVariant = true
		//	}
		//	if status == "partial" {
		//		isPartial = true
		//	}
		//	if status == "both" {
		//		isVariant = true
		//		isPartial = true
		//	}
		//}
		//
		//if !isVariant && !isPartial {
		//	return "", nil
		//}
		//if isVariant {
		//	return "variant", nil
		//}
		//if isPartial {
		//	return "partial", nil
		//}
		//
		////return "", fmt.Errorf("matches as both partial and variant")
		//return "both", nil

		// if we're not comparing arg names, get the two lists of types
		var isVariant, isPartial bool
		for i := 0; i < len(obj.Ord); i++ {
			t1, ok := obj.Map[obj.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t1 == nil {
				panic("malformed func arg")
			}

			t2, ok := typ.Map[typ.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			if t2 == nil {
				isPartial = true
				continue
			}

			status, err := t1.ComplexCmp(t2)
			if err != nil {
				return "", err
			}
			if status == "variant" {
				isVariant = true
			}
			if status == "partial" {
				isPartial = true
			}
			if status == "both" {
				isVariant = true
				isPartial = true
			}
		}

		//if obj.Out != nil && typ.Out != nil { // let a nil obj.Out in
		if typ.Out != nil { // let a nil obj.Out in
			status, err := obj.Out.ComplexCmp(typ.Out)
			if err != nil {
				return "", err
			}
			if status == "variant" {
				isVariant = true
			}
			if status == "partial" {
				isPartial = true
			}
			if status == "both" {
				isVariant = true
				isPartial = true
			}

		} else if obj.Out != nil {
			// TODO: technically, typ.Out could be unspecified, then
			// this is a Cmp fail, not an isPartial = true, but then
			// we'd have to support functions without a return value
			// since we are functional, it is not a major problem...
			isPartial = true
		}
		//} else if typ.Out != nil { // solve this in the above ComplexCmp instead!
		//	return "", fmt.Errorf("can't cmp from a nil type")
		//}

		if !isVariant && !isPartial {
			return "", nil
		}
		if isVariant {
			return "variant", nil
		}
		if isPartial {
			return "partial", nil
		}

		//return "", fmt.Errorf("matches as both partial and variant")
		return "both", nil
	}

	return "", fmt.Errorf("unknown kind: %+v", obj.Kind)
}

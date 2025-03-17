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

package types

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/disjoint"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// StructTag is the key we use in struct field names for key mapping.
	StructTag = "lang"

	// MaxInt8 is 127. It's max uint8: ^uint8(0), then we >> 1 for max int8.
	MaxInt8 = int((^uint8(0)) >> 1)
)

// Basic types defined here as a convenience for use with Type.Cmp(X).
var (
	TypeBool    = NewType("bool")
	TypeStr     = NewType("str")
	TypeInt     = NewType("int")
	TypeFloat   = NewType("float")
	TypeListStr = NewType("[]str")
	TypeVariant = NewType("variant")
)

// The Kind represents the base type of each value.
type Kind int // this used to be called Type

// Each Kind represents a type in the language type system.
const (
	// NOTE: Make sure you add entries to stringer.go if you add something.
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

	KindUnification = Kind(MaxInt8) // keep this last
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

	// unification variable (question mark, eg ?1, ?2)
	Uni *Elem // if Kind == Unification (optional) use Uni only
}

// Elem is the type used for the unification variable in the Uni field of Type.
// We create this alias here to avoid needing to write *disjoint.Elem[*Type] all
// over. This is a golang type alias. These should be created with NewElem.
type Elem = disjoint.Elem[*Type]

// NewElem creates a new set with one element and returns the sole element (the
// representative element) of that set.
func NewElem() *Elem {
	return disjoint.NewElem[*Type]()
}

// TypeOf takes a reflect.Type and returns an equivalent *Type. It removes any
// pointers since our language does not support pointers. It returns nil if it
// cannot represent the type in our type system. Common examples of things it
// cannot express include reflect.Invalid, reflect.Interface, Reflect.Complex128
// and more. It is not reversible because some information may be either added
// or lost. For example, reflect.Array and reflect.Slice are both converted to a
// Type of KindList, and KindFunc names the arguments of a func sequentially.
// The lossy inverse of this is Reflect.
func TypeOf(t reflect.Type) (*Type, error) {
	opts := []TypeOfOption{
		StructTagOpt(StructTag),
		StrictStructTagOpt(false),
		SkipBadStructFieldsOpt(false),
		SkipPrivateFieldsOpt(false),
		AllowInterfaceTypeOpt(false),
	}
	return ConfigurableTypeOf(t, opts...)
}

// ResTypeOf is almost identical to TypeOf, except it behaves slightly
// differently so that it can return what is needed for resources.
func ResTypeOf(t reflect.Type) (*Type, error) {
	opts := []TypeOfOption{
		StructTagOpt(StructTag),
		StrictStructTagOpt(true),
		SkipBadStructFieldsOpt(true),
		SkipPrivateFieldsOpt(true),
		AllowInterfaceTypeOpt(true),
	}
	return ConfigurableTypeOf(t, opts...)
}

// TypeOfOption is a type that can be used to configure the ConfigurableTypeOf
// function.
type TypeOfOption func(*typeOfOptions)

// typeOfOptions represents the different possible configurable options.
type typeOfOptions struct {
	structTag           string
	strictStructTag     bool
	skipBadStructFields bool
	skipPrivateFields   bool
	allowInterfaceType  bool
	// TODO: add more options
}

// StructTagOpt specifies whether we should skip over struct fields that errored
// when we tried to find their type. This is used by ResTypeOf.
func StructTagOpt(structTag string) TypeOfOption {
	return func(opt *typeOfOptions) {
		opt.structTag = structTag
	}
}

// StrictStructTagOpt specifies whether we require that a struct tag be present
// to be able to use the field. If false, then the field is skipped if it is
// missing a struct tag.
func StrictStructTagOpt(strictStructTag bool) TypeOfOption {
	return func(opt *typeOfOptions) {
		opt.strictStructTag = strictStructTag
	}
}

// SkipBadStructFieldsOpt specifies whether we should skip over struct fields
// that errored when we tried to find their type. This is used by ResTypeOf.
func SkipBadStructFieldsOpt(skipBadStructFields bool) TypeOfOption {
	return func(opt *typeOfOptions) {
		opt.skipBadStructFields = skipBadStructFields
	}
}

// SkipPrivateFieldsOpt specifies whether we should skip over struct fields that
// are private or unexported. This is used by ResTypeOf.
func SkipPrivateFieldsOpt(skipPrivateFields bool) TypeOfOption {
	return func(opt *typeOfOptions) {
		opt.skipPrivateFields = skipPrivateFields
	}
}

// AllowInterfaceTypeOpt specifies whether we should allow matching on an
// interface kind. This is used by ResTypeOf.
func AllowInterfaceTypeOpt(allowInterfaceType bool) TypeOfOption {
	return func(opt *typeOfOptions) {
		opt.allowInterfaceType = allowInterfaceType
	}
}

// ConfigurableTypeOf is a configurable version of the TypeOf function to avoid
// repeating code for the different variants of it that we want.
func ConfigurableTypeOf(t reflect.Type, opts ...TypeOfOption) (*Type, error) {
	options := &typeOfOptions{ // default options
		structTag:           "",
		strictStructTag:     false,
		skipBadStructFields: false,
		skipPrivateFields:   false,
		allowInterfaceType:  false,
	}
	for _, optionFunc := range opts { // apply the options
		optionFunc(options)
	}
	if options.strictStructTag && options.structTag == "" {
		return nil, fmt.Errorf("strict struct tag is set and struct tag is empty")
	}

	typ := t
	kind := typ.Kind()
	for kind == reflect.Ptr {
		typ = typ.Elem() // un-nest one pointer
		kind = typ.Kind()
	}

	// Special cases:
	if reflect.TypeOf(net.HardwareAddr{}) == typ {
		return &Type{
			Kind: KindStr,
		}, nil
	}
	// TODO: net/url.URL, time.Duration, etc. Note: avoid net/mail.Address

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
		val, err := ConfigurableTypeOf(typ.Elem(), opts...)
		if err != nil {
			return nil, err
		}

		return &Type{
			Kind: KindList,
			Val:  val,
		}, nil

	case reflect.Map:
		key, err := ConfigurableTypeOf(typ.Key(), opts...) // Key returns a map type's key type.
		if err != nil {
			return nil, err
		}
		val, err := ConfigurableTypeOf(typ.Elem(), opts...) // Elem returns a type's element type.
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
			if options.skipPrivateFields && !field.IsExported() { // prevent infinite recursion
				continue
			}
			tt, err := ConfigurableTypeOf(field.Type, opts...)
			if err != nil {
				if options.skipBadStructFields {
					continue // skip over bad fields!
				}
				return nil, err
			}
			// TODO: should we skip over fields with field.Anonymous ?

			// if struct field has a `lang:""` tag, use that instead of the struct field name
			fieldName := field.Name
			if options.structTag != "" {
				if alias, ok := field.Tag.Lookup(options.structTag); ok {
					fieldName = alias
				} else if options.strictStructTag {
					continue
				}
			}

			if util.StrInList(fieldName, ord) {
				return nil, fmt.Errorf("duplicate struct field name: `%s` alias: `%s`", field.Name, fieldName)
			}

			m[fieldName] = tt
			ord = append(ord, fieldName) // in order
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
			tt, err := ConfigurableTypeOf(typ.In(i), opts...)
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
			out, err = ConfigurableTypeOf(typ.Out(0), opts...)
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
	case reflect.Interface:
		if !options.allowInterfaceType {
			return nil, fmt.Errorf("unable to represent type of %s without AllowInterfaceTypeOpt", typ.String())
		}

		return &Type{
			Kind: KindVariant,
			Var:  nil, // TODO: can we set this?
		}, nil

	default:
		return nil, fmt.Errorf("unable to represent type of %s", typ.String())
	}
}

// NewType creates the Type from the string representation.
func NewType(s string) *Type {
	table := make(map[uint]*Elem)
	return newType(s, table)
}

// newType creates the Type from the string representation. This private version
// takes a table so that we can collect unification variables as we see them and
// return a type with correctly unified unification variables.
func newType(s string, table map[uint]*Elem) *Type {
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
		val := newType(s[len("[]"):], table)
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

		key := newType(strings.Trim(s[:found], " "), table)
		if key == nil {
			return nil
		}
		val := newType(strings.Trim(s[found+1:], " "), table)
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

			typ := newType(strings.Trim(s[:found+1-trim], " "), table)
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
			// XXX: util.NumToAlpha ?
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

			typ := newType(strings.Trim(s[:found+1-trim], " "), table)
			if typ == nil {
				return nil
			}
			tmap[key] = typ // add type
			s = s[found+1:] // what's left?
		}

		// return type
		var tail *Type
		if out != "" { // allow functions with no return type (in parser)
			tail = newType(out, table)
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

	// KindUnification
	if strings.HasPrefix(s, "?") {
		// find end of number...
		var length = 0 // number of digits
		for i := len("?"); i < len(s); i++ {
			c := s[i]
			if length == 0 && c == '0' {
				return nil // can't start with a zero
			}

			// Check manually because strconv.ParseUint accepts ^0x.
			if '0' <= c && c <= '9' {
				length++
				continue
			}
			return nil // invalid char
		}

		v := s[len("?") : len("?")+length]
		n, err := strconv.ParseUint(v, 10, 32) // base 10, 32 bits
		if err != nil {
			return nil // programming error or overflow
		}
		num := uint(n)

		// XXX: Should we instead always return new unification
		// variables, but call .Union() on all of the ones that have the
		// same integer? Sam says they are equivalent.
		uni, exists := table[num]
		if !exists {
			uni = NewElem()  // unification variable, eg: ?1
			table[num] = uni // store
		}

		// return a new type, may have an existing unification variable
		return &Type{
			Kind: KindUnification,
			Uni:  uni, // unification variable, eg: ?1
		}
	}

	return nil // error (this also matches the empty string as input)
}

// New creates a new Value of this type. It will represent the "zero" value. It
// panics if you give it a malformed type.
func (obj *Type) New() Value {
	if obj == nil {
		panic("malformed type")
	}
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
	case KindUnification:
		panic("can't make new value from unification variable kind")
	}
	panic("malformed type")
}

// String returns the textual representation for this type.
func (obj *Type) String() string {
	table := make(map[*Elem]uint)
	return obj.string(table)
}

// string returns the textual representation for this type. This is a private
// helper function that is used by the real String function.
func (obj *Type) string(table map[*Elem]uint) string {
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
		return "[]" + obj.Val.string(table)

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}
		return fmt.Sprintf("map{%s: %s}", obj.Key.string(table), obj.Val.string(table))

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
			s[i] = fmt.Sprintf("%s %s", k, t.string(table))
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

			// We need to print function arg names for Copy() to use
			// the String() hack here and avoid erasing them here!
			//s[i] = t.string(table)
			s[i] = fmt.Sprintf("%s %s", k, t.string(table)) // strict
		}
		var out string
		if obj.Out != nil {
			out = fmt.Sprintf(" %s", obj.Out.string(table))
		}
		return fmt.Sprintf("func(%s)%s", strings.Join(s, ", "), out)

	case KindVariant:
		return "variant"

	case KindUnification:
		if obj.Uni == nil {
			panic("malformed unification variable")
		}

		// XXX: Should we instead run .IsConnected() on the two Elem
		// unification variables to determine if they should have the
		// same integer representation when printing them?
		num, exists := table[obj.Uni]
		if !exists {
			for _, n := range table {
				num = max(num, n)
			}
			num++                // add 1
			table[obj.Uni] = num // store
		}

		//fmt.Printf("?%d: %p\n", int(num), obj.Uni.Find()) // debug
		return "?" + strconv.Itoa(int(num))
	}

	panic("malformed type")
}

// Cmp compares this type to another.
func (obj *Type) Cmp(typ *Type) error {
	table1 := make(map[*Elem]uint) // for obj
	table2 := make(map[*Elem]uint) // for typ
	return obj.cmp(typ, table1, table2)
}

// cmp compares this type to another. This is a private helper function that is
// used by the real Cmp function.
func (obj *Type) cmp(typ *Type, table1, table2 map[*Elem]uint) error {
	if obj == nil || typ == nil {
		return fmt.Errorf("cannot compare to nil")
	}

	// TODO: is this correct?
	// recurse into variants if we want base type comparisons
	//if obj.Kind == KindVariant {
	//	return obj.Var.cmp(t, table1, table2)
	//}
	//if t.Kind == KindVariant {
	//	return obj.cmp(t.Var, table1, table2)
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
		return obj.Val.cmp(typ.Val, table1, table2)

	case KindMap:
		if obj.Key == nil || obj.Val == nil || typ.Key == nil || typ.Val == nil {
			panic("malformed map type")
		}
		kerr := obj.Key.cmp(typ.Key, table1, table2)
		verr := obj.Val.cmp(typ.Val, table1, table2)
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
			if err := t1.cmp(t2, table1, table2); err != nil {
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
		//	if err := t1.cmp(t2, table1, table2); err != nil {
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

			if err := t1.cmp(t2, table1, table2); err != nil {
				return err
			}
		}

		if obj.Out != nil || typ.Out != nil {
			if err := obj.Out.cmp(typ.Out, table1, table2); err != nil {
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

	// used for testing
	case KindUnification:
		if obj.Uni == nil || typ.Uni == nil {
			panic("malformed unification variable")
		}

		// If both types store and lookup variables symmetrically and in
		// the same order, then the count's should also match.
		// XXX: Should we instead run .IsConnected() on the two Elem
		// unification variables to determine if they should have the
		// same integer representation when printing them?
		num1, exists := table1[obj.Uni]
		if !exists {
			for _, n := range table1 {
				num1 = max(num1, n)
			}
			num1++                 // add 1
			table1[obj.Uni] = num1 // store
		}

		num2, exists := table2[typ.Uni]
		if !exists {
			for _, n := range table2 {
				num2 = max(num2, n)
			}
			num2++                 // add 1
			table2[typ.Uni] = num2 // store
		}
		if num1 != num2 {
			return fmt.Errorf("unbalanced unification variables")
		}
		return nil
	}
	return fmt.Errorf("unknown kind")
}

// Copy copies this type so that inplace modification won't affect the original.
func (obj *Type) Copy() *Type {
	// String() needs to print function arg names or they'd get erased here!
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
			if strings.Title(k) != k { // is exported?
				//k = strings.Title(k) // TODO: is this helpful?
				// reflect.StructOf would panic on anything unexported
				panic(fmt.Sprintf("struct has unexported field: %s", k))
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

	case KindUnification:
		return false // TODO: Do we want to panic here instead?
	}

	panic("malformed type")
}

// HasUni tells us if the type contains any unification variables.
func (obj *Type) HasUni() bool {
	if obj == nil {
		return false
	}
	if obj.Uni != nil {
		return true // found it (by this method)
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
		return obj.Val.HasUni()

	case KindMap:
		if obj.Key == nil || obj.Val == nil {
			panic("malformed map type")
		}
		return obj.Key.HasUni() || obj.Val.HasUni()

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
			if t.HasUni() {
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
			if t.HasUni() {
				return true
			}
		}
		if obj.Out != nil {
			if obj.Out.HasUni() {
				return true
			}
		}
		return false

	case KindVariant:
		return obj.Var.HasUni()

	case KindUnification:
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
// This now also supports comparing a partial type to a variant type as well...
// TODO: Should we support KindUnification somehow?
func (obj *Type) ComplexCmp(typ *Type) (string, error) {
	// match simple "placeholder" variants... skip variants w/ sub types
	isVariant := func(t *Type) bool { return t != nil && t.Kind == KindVariant && t.Var == nil }

	if obj == nil && typ == nil {
		return "partial", nil // compatible :)
	}
	if isVariant(obj) && isVariant(typ) {
		return "variant", nil // compatible :)
	}

	if obj == nil && isVariant(typ) { // partial vs variant
		return "both", nil // compatible :)
	}
	if isVariant(obj) && typ == nil { // variant vs partial
		return "both", nil // compatible :)
	}

	if obj == nil || typ == nil { // at least one is partial
		return "partial", nil // compatible :)
	}
	if isVariant(obj) || isVariant(typ) { // at least one is variant
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
		return obj.Val.ComplexCmp(typ.Val)

	case KindMap:
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
		if isVariant && !isPartial {
			return "variant", nil
		}
		if isPartial && !isVariant {
			return "partial", nil
		}

		return "both", nil

	case KindStruct: // {a bool; b int}
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
		if isVariant && !isPartial {
			return "variant", nil
		}
		if isPartial && !isVariant {
			return "partial", nil
		}

		return "both", nil

	case KindFunc:
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
		//if isVariant && !isPartial {
		//	return "variant", nil
		//}
		//if isPartial && !isVariant {
		//	return "partial", nil
		//}
		//
		//return "both", nil

		// if we're not comparing arg names, get the two lists of types
		var isVariant, isPartial bool
		for i := 0; i < len(obj.Ord); i++ {
			t1, ok := obj.Map[obj.Ord[i]]
			if !ok {
				panic("malformed func order")
			}
			t2, ok := typ.Map[typ.Ord[i]]
			if !ok {
				panic("malformed func order")
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

		// NOTE: Technically, .Out could be unspecified, then this is a
		// Cmp fail, not an isPartial = true, but then we'd have to
		// support functions without a return value. Since we are
		// functional, it is not a major problem...

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

		if !isVariant && !isPartial {
			return "", nil
		}
		if isVariant && !isPartial {
			return "variant", nil
		}
		if isPartial && !isVariant {
			return "partial", nil
		}

		return "both", nil
	}

	return "", fmt.Errorf("unknown kind: %+v", obj.Kind)
}

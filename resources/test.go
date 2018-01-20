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

package resources

import (
	"fmt"
	"log"
	"reflect"
)

func init() {
	RegisterResource("test", func() Res { return &TestRes{} })
}

// TestRes is a resource that is mostly harmless and is used for internal tests.
type TestRes struct {
	BaseRes `lang:"" yaml:",inline"`

	Bool bool   `lang:"bool" yaml:"bool"`
	Str  string `lang:"str" yaml:"str"` // can't name it String because of String()

	Int   int   `lang:"int" yaml:"int"`
	Int8  int8  `lang:"int8" yaml:"int8"`
	Int16 int16 `lang:"int16" yaml:"int16"`
	Int32 int32 `lang:"int32" yaml:"int32"`
	Int64 int64 `lang:"int64" yaml:"int64"`

	Uint   uint   `lang:"uint" yaml:"uint"`
	Uint8  uint8  `lang:"uint8" yaml:"uint8"`
	Uint16 uint16 `lang:"uint16" yaml:"uint16"`
	Uint32 uint32 `lang:"uint32" yaml:"uint32"`
	Uint64 uint64 `lang:"uint64" yaml:"uint64"`

	//Uintptr uintptr `yaml:"uintptr"`
	Byte byte `lang:"byte" yaml:"byte"` // alias for uint8
	Rune rune `lang:"rune" yaml:"rune"` // alias for int32, represents a Unicode code point

	Float32    float32    `lang:"float32" yaml:"float32"`
	Float64    float64    `lang:"float64" yaml:"float64"`
	Complex64  complex64  `lang:"complex64" yaml:"complex64"`
	Complex128 complex128 `lang:"complex128" yaml:"complex128"`

	BoolPtr   *bool   `lang:"boolptr" yaml:"bool_ptr"`
	StringPtr *string `lang:"stringptr" yaml:"string_ptr"` // TODO: tag name?
	Int64Ptr  *int64  `lang:"int64ptr" yaml:"int64ptr"`
	Int8Ptr   *int8   `lang:"int8ptr" yaml:"int8ptr"`
	Uint8Ptr  *uint8  `lang:"uint8ptr" yaml:"uint8ptr"`

	// probably makes no sense, but is legal
	Int8PtrPtrPtr ***int8 `lang:"int8ptrptrptr" yaml:"int8ptrptrptr"`

	SliceString []string          `lang:"slicestring" yaml:"slicestring"`
	MapIntFloat map[int64]float64 `lang:"mapintfloat" yaml:"mapintfloat"`
	MixedStruct struct {
		somebool  bool
		somestr   string
		someint   int64
		somefloat float64
	} `lang:"mixedstruct" yaml:"mixedstruct"`
	Interface interface{} `lang:"interface" yaml:"interface"`

	AnotherStr string `lang:"anotherstr" yaml:"anotherstr"`

	ValidateBool  bool   `lang:"validatebool" yaml:"validate_bool"`   // set to true to cause a validate error
	ValidateError string `lang:"validateerror" yaml:"validate_error"` // set to cause a validate error
	AlwaysGroup   bool   `lang:"alwaysgroup" yaml:"always_group"`     // set to true to cause auto grouping
	CompareFail   bool   `lang:"comparefail" yaml:"compare_fail"`     // will compare fail?

	// TODO: add more fun properties!

	Comment string `lang:"comment" yaml:"comment"`
}

// Default returns some sensible defaults for this resource.
func (obj *TestRes) Default() Res {
	return &TestRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *TestRes) Validate() error {
	if obj.ValidateBool {
		return fmt.Errorf("the validate param was set to true")
	}
	if s := obj.ValidateError; s != "" {
		return fmt.Errorf("the validate error param was set to: %s", s)
	}
	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *TestRes) Init() error {
	return obj.BaseRes.Init() // call base init, b/c we're overriding
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TestRes) Watch() error {
	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false // send event?
	var exit *error
	for {
		select {
		case event := <-obj.Events():
			// we avoid sending events on unpause
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// CheckApply method for Test resource. Does nothing, returns happy!
func (obj *TestRes) CheckApply(apply bool) (checkOK bool, err error) {
	log.Printf("%s: CheckApply: %t", obj, apply)
	if obj.Refresh() {
		log.Printf("%s: Received a notification!", obj)
	}

	log.Printf("%s: Bool:          %v", obj, obj.Bool)
	log.Printf("%s: Str:           %v", obj, obj.Str)

	log.Printf("%s: Int:           %v", obj, obj.Int)
	log.Printf("%s: Int8:          %v", obj, obj.Int8)
	log.Printf("%s: Int16:         %v", obj, obj.Int16)
	log.Printf("%s: Int32:         %v", obj, obj.Int32)
	log.Printf("%s: Int64:         %v", obj, obj.Int64)

	log.Printf("%s: Uint:          %v", obj, obj.Uint)
	log.Printf("%s: Uint8:         %v", obj, obj.Uint)
	log.Printf("%s: Uint16:        %v", obj, obj.Uint)
	log.Printf("%s: Uint32:        %v", obj, obj.Uint)
	log.Printf("%s: Uint64:        %v", obj, obj.Uint)

	//log.Printf("%s: Uintptr:       %v", obj, obj.Uintptr)
	log.Printf("%s: Byte:          %v", obj, obj.Byte)
	log.Printf("%s: Rune:          %v", obj, obj.Rune)

	log.Printf("%s: Float32:       %v", obj, obj.Float32)
	log.Printf("%s: Float64:       %v", obj, obj.Float64)
	log.Printf("%s: Complex64:     %v", obj, obj.Complex64)
	log.Printf("%s: Complex128:    %v", obj, obj.Complex128)

	log.Printf("%s: BoolPtr:       %v", obj, obj.BoolPtr)
	log.Printf("%s: StringPtr:     %v", obj, obj.StringPtr)
	log.Printf("%s: Int64Ptr:      %v", obj, obj.Int64Ptr)
	log.Printf("%s: Int8Ptr:       %v", obj, obj.Int8Ptr)
	log.Printf("%s: Uint8Ptr:      %v", obj, obj.Uint8Ptr)

	log.Printf("%s: Int8PtrPtrPtr: %v", obj, obj.Int8PtrPtrPtr)

	log.Printf("%s: SliceString:   %v", obj, obj.SliceString)
	log.Printf("%s: MapIntFloat:   %v", obj, obj.MapIntFloat)
	log.Printf("%s: MixedStruct:   %v", obj, obj.MixedStruct)
	log.Printf("%s: Interface:     %v", obj, obj.Interface)

	log.Printf("%s: AnotherStr:    %v", obj, obj.AnotherStr)

	return true, nil // state is always okay
}

// TestUID is the UID struct for TestRes.
type TestUID struct {
	BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *TestRes) UIDs() []ResUID {
	x := &TestUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *TestRes) GroupCmp(r Res) bool {
	_, ok := r.(*TestRes)
	if !ok {
		return false
	}
	return obj.AlwaysGroup // grouped together if we were asked to
}

// Compare two resources and return if they are equivalent.
func (obj *TestRes) Compare(r Res) bool {
	// we can only compare TestRes to others of the same resource kind
	res, ok := r.(*TestRes)
	if !ok {
		return false
	}
	// calling base Compare is probably unneeded for the test res, but do it
	if !obj.BaseRes.Compare(res) { // call base Compare
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	if obj.CompareFail || res.CompareFail {
		return false
	}

	// TODO: yes, I know the long manual version is absurd, but I couldn't
	// get these to work :(
	//if !reflect.DeepEqual(obj, res) { // is broken :/
	//if diff := pretty.Compare(obj, res); diff != "" { // causes stack overflow
	//	return false
	//}

	if obj.Bool != res.Bool {
		return false
	}
	if obj.Str != res.Str {
		return false
	}

	if obj.Int != res.Int {
		return false
	}
	if obj.Int8 != res.Int8 {
		return false
	}
	if obj.Int16 != res.Int16 {
		return false
	}
	if obj.Int32 != res.Int32 {
		return false
	}
	if obj.Int64 != res.Int64 {
		return false
	}

	if obj.Uint != res.Uint {
		return false
	}
	if obj.Uint8 != res.Uint8 {
		return false
	}
	if obj.Uint16 != res.Uint16 {
		return false
	}
	if obj.Uint32 != res.Uint32 {
		return false
	}
	if obj.Uint64 != res.Uint64 {
		return false
	}

	//if obj.Uintptr
	if obj.Byte != res.Byte {
		return false
	}
	if obj.Rune != res.Rune {
		return false
	}

	if obj.Float32 != res.Float32 {
		return false
	}
	if obj.Float64 != res.Float64 {
		return false
	}
	if obj.Complex64 != res.Complex64 {
		return false
	}
	if obj.Complex128 != res.Complex128 {
		return false
	}

	if (obj.BoolPtr == nil) != (res.BoolPtr == nil) { // xor
		return false
	}
	if obj.BoolPtr != nil && res.BoolPtr != nil {
		if *obj.BoolPtr != *res.BoolPtr { // compare
			return false
		}
	}
	if (obj.StringPtr == nil) != (res.StringPtr == nil) { // xor
		return false
	}
	if obj.StringPtr != nil && res.StringPtr != nil {
		if *obj.StringPtr != *res.StringPtr { // compare
			return false
		}
	}
	if (obj.Int64Ptr == nil) != (res.Int64Ptr == nil) { // xor
		return false
	}
	if obj.Int64Ptr != nil && res.Int64Ptr != nil {
		if *obj.Int64Ptr != *res.Int64Ptr { // compare
			return false
		}
	}
	if (obj.Int8Ptr == nil) != (res.Int8Ptr == nil) { // xor
		return false
	}
	if obj.Int8Ptr != nil && res.Int8Ptr != nil {
		if *obj.Int8Ptr != *res.Int8Ptr { // compare
			return false
		}
	}
	if (obj.Uint8Ptr == nil) != (res.Uint8Ptr == nil) { // xor
		return false
	}
	if obj.Uint8Ptr != nil && res.Uint8Ptr != nil {
		if *obj.Uint8Ptr != *res.Uint8Ptr { // compare
			return false
		}
	}

	if !reflect.DeepEqual(obj.Int8PtrPtrPtr, res.Int8PtrPtrPtr) {
		return false
	}

	if !reflect.DeepEqual(obj.SliceString, res.SliceString) {
		return false
	}
	if !reflect.DeepEqual(obj.MapIntFloat, res.MapIntFloat) {
		return false
	}
	if !reflect.DeepEqual(obj.MixedStruct, res.MixedStruct) {
		return false
	}
	if !reflect.DeepEqual(obj.Interface, res.Interface) {
		return false
	}

	if obj.AnotherStr != res.AnotherStr {
		return false
	}

	if obj.ValidateBool != res.ValidateBool {
		return false
	}
	if obj.ValidateError != res.ValidateError {
		return false
	}
	if obj.AlwaysGroup != res.AlwaysGroup {
		return false
	}

	if obj.Comment != res.Comment {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *TestRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes TestRes // indirection to avoid infinite recursion

	def := obj.Default()      // get the default
	res, ok := def.(*TestRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to TestRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = TestRes(raw) // restore from indirection with type conversion!
	return nil
}

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

package resources

import (
	"fmt"
	"reflect"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
)

func init() {
	engine.RegisterResource("test", func() engine.Res { return &TestRes{} })
}

// TestRes is a resource that is mostly harmless and is used for internal tests.
type TestRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Groupable
	traits.Refreshable
	traits.Sendable
	traits.Recvable

	init *engine.Init

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
	SendValue     string `lang:"sendvalue" yaml:"send_value"`         // what value should we send?

	// TODO: add more fun properties!

	Comment string `lang:"comment" yaml:"comment"`
}

// Default returns some sensible defaults for this resource.
func (obj *TestRes) Default() engine.Res {
	return &TestRes{}
}

// Validate if the params passed in are valid data.
func (obj *TestRes) Validate() error {
	if obj.ValidateBool {
		return fmt.Errorf("the validate param was set to true")
	}
	if s := obj.ValidateError; s != "" {
		return fmt.Errorf("the validate error param was set to: %s", s)
	}
	return nil
}

// Init runs some startup code for this resource.
func (obj *TestRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *TestRes) Close() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TestRes) Watch() error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-obj.init.Done: // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply method for Test resource. Does nothing, returns happy!
func (obj *TestRes) CheckApply(apply bool) (bool, error) {
	for key, val := range obj.init.Recv() {
		obj.init.Logf("CheckApply: Received `%s`, changed: %t", key, val.Changed)
	}

	if obj.init.Refresh() {
		obj.init.Logf("Received a notification!")
	}

	obj.init.Logf("%s: Bool:          %v", obj, obj.Bool)
	obj.init.Logf("%s: Str:           %v", obj, obj.Str)

	obj.init.Logf("%s: Int:           %v", obj, obj.Int)
	obj.init.Logf("%s: Int8:          %v", obj, obj.Int8)
	obj.init.Logf("%s: Int16:         %v", obj, obj.Int16)
	obj.init.Logf("%s: Int32:         %v", obj, obj.Int32)
	obj.init.Logf("%s: Int64:         %v", obj, obj.Int64)

	obj.init.Logf("%s: Uint:          %v", obj, obj.Uint)
	obj.init.Logf("%s: Uint8:         %v", obj, obj.Uint)
	obj.init.Logf("%s: Uint16:        %v", obj, obj.Uint)
	obj.init.Logf("%s: Uint32:        %v", obj, obj.Uint)
	obj.init.Logf("%s: Uint64:        %v", obj, obj.Uint)

	//obj.init.Logf("%s: Uintptr:       %v", obj, obj.Uintptr)
	obj.init.Logf("%s: Byte:          %v", obj, obj.Byte)
	obj.init.Logf("%s: Rune:          %v", obj, obj.Rune)

	obj.init.Logf("%s: Float32:       %v", obj, obj.Float32)
	obj.init.Logf("%s: Float64:       %v", obj, obj.Float64)
	obj.init.Logf("%s: Complex64:     %v", obj, obj.Complex64)
	obj.init.Logf("%s: Complex128:    %v", obj, obj.Complex128)

	obj.init.Logf("%s: BoolPtr:       %v", obj, obj.BoolPtr)
	obj.init.Logf("%s: StringPtr:     %v", obj, obj.StringPtr)
	obj.init.Logf("%s: Int64Ptr:      %v", obj, obj.Int64Ptr)
	obj.init.Logf("%s: Int8Ptr:       %v", obj, obj.Int8Ptr)
	obj.init.Logf("%s: Uint8Ptr:      %v", obj, obj.Uint8Ptr)

	obj.init.Logf("%s: Int8PtrPtrPtr: %v", obj, obj.Int8PtrPtrPtr)

	obj.init.Logf("%s: SliceString:   %v", obj, obj.SliceString)
	obj.init.Logf("%s: MapIntFloat:   %v", obj, obj.MapIntFloat)
	obj.init.Logf("%s: MixedStruct:   %v", obj, obj.MixedStruct)
	obj.init.Logf("%s: Interface:     %v", obj, obj.Interface)

	obj.init.Logf("%s: AnotherStr:    %v", obj, obj.AnotherStr)

	// send
	hello := obj.SendValue
	if err := obj.init.Send(&TestSends{
		Hello:  &hello,
		Answer: 42,
	}); err != nil {
		return false, err
	}

	return true, nil // state is always okay
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *TestRes) Cmp(r engine.Res) error {
	// we can only compare TestRes to others of the same resource kind
	res, ok := r.(*TestRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}
	//if obj.Name != res.Name {
	//	return false
	//}

	if obj.CompareFail || res.CompareFail {
		return fmt.Errorf("the CompareFail is true")
	}

	// TODO: yes, I know the long manual version is absurd, but I couldn't
	// get these to work :(
	//if !reflect.DeepEqual(obj, res) { // is broken :/
	//if diff := pretty.Compare(obj, res); diff != "" { // causes stack overflow
	//	return false
	//}

	if obj.Bool != res.Bool {
		return fmt.Errorf("the Bool differs")
	}
	if obj.Str != res.Str {
		return fmt.Errorf("the Str differs")
	}

	if obj.Int != res.Int {
		return fmt.Errorf("the Str differs")
	}
	if obj.Int8 != res.Int8 {
		return fmt.Errorf("the Int8 differs")
	}
	if obj.Int16 != res.Int16 {
		return fmt.Errorf("the Int16 differs")
	}
	if obj.Int32 != res.Int32 {
		return fmt.Errorf("the Int32 differs")
	}
	if obj.Int64 != res.Int64 {
		return fmt.Errorf("the Int64 differs")
	}

	if obj.Uint != res.Uint {
		return fmt.Errorf("the Uint differs")
	}
	if obj.Uint8 != res.Uint8 {
		return fmt.Errorf("the Uint8 differs")
	}
	if obj.Uint16 != res.Uint16 {
		return fmt.Errorf("the Uint16 differs")
	}
	if obj.Uint32 != res.Uint32 {
		return fmt.Errorf("the Uint32 differs")
	}
	if obj.Uint64 != res.Uint64 {
		return fmt.Errorf("the Uint64 differs")
	}

	//if obj.Uintptr
	if obj.Byte != res.Byte {
		return fmt.Errorf("the Byte differs")
	}
	if obj.Rune != res.Rune {
		return fmt.Errorf("the Rune differs")
	}

	if obj.Float32 != res.Float32 {
		return fmt.Errorf("the Float32 differs")
	}
	if obj.Float64 != res.Float64 {
		return fmt.Errorf("the Float64 differs")
	}
	if obj.Complex64 != res.Complex64 {
		return fmt.Errorf("the Complex64 differs")
	}
	if obj.Complex128 != res.Complex128 {
		return fmt.Errorf("the Complex128 differs")
	}

	if (obj.BoolPtr == nil) != (res.BoolPtr == nil) { // xor
		return fmt.Errorf("the BoolPtr differs")
	}
	if obj.BoolPtr != nil && res.BoolPtr != nil {
		if *obj.BoolPtr != *res.BoolPtr { // compare
			return fmt.Errorf("the BoolPtr differs")
		}
	}
	if (obj.StringPtr == nil) != (res.StringPtr == nil) { // xor
		return fmt.Errorf("the StringPtr differs")
	}
	if obj.StringPtr != nil && res.StringPtr != nil {
		if *obj.StringPtr != *res.StringPtr { // compare
			return fmt.Errorf("the StringPtr differs")
		}
	}
	if (obj.Int64Ptr == nil) != (res.Int64Ptr == nil) { // xor
		return fmt.Errorf("the Int64Ptr differs")
	}
	if obj.Int64Ptr != nil && res.Int64Ptr != nil {
		if *obj.Int64Ptr != *res.Int64Ptr { // compare
			return fmt.Errorf("the Int64Ptr differs")
		}
	}
	if (obj.Int8Ptr == nil) != (res.Int8Ptr == nil) { // xor
		return fmt.Errorf("the Int8Ptr differs")
	}
	if obj.Int8Ptr != nil && res.Int8Ptr != nil {
		if *obj.Int8Ptr != *res.Int8Ptr { // compare
			return fmt.Errorf("the Int8Ptr differs")
		}
	}
	if (obj.Uint8Ptr == nil) != (res.Uint8Ptr == nil) { // xor
		return fmt.Errorf("the Uint8Ptr differs")
	}
	if obj.Uint8Ptr != nil && res.Uint8Ptr != nil {
		if *obj.Uint8Ptr != *res.Uint8Ptr { // compare
			return fmt.Errorf("the Uint8Ptr differs")
		}
	}

	if !reflect.DeepEqual(obj.Int8PtrPtrPtr, res.Int8PtrPtrPtr) {
		return fmt.Errorf("the Int8PtrPtrPtr differs")
	}

	if !reflect.DeepEqual(obj.SliceString, res.SliceString) {
		return fmt.Errorf("the SliceString differs")
	}
	if !reflect.DeepEqual(obj.MapIntFloat, res.MapIntFloat) {
		return fmt.Errorf("the MapIntFloat differs")
	}
	if !reflect.DeepEqual(obj.MixedStruct, res.MixedStruct) {
		return fmt.Errorf("the MixedStruct differs")
	}
	if !reflect.DeepEqual(obj.Interface, res.Interface) {
		return fmt.Errorf("the Interface differs")
	}

	if obj.AnotherStr != res.AnotherStr {
		return fmt.Errorf("the AnotherStr differs")
	}

	if obj.ValidateBool != res.ValidateBool {
		return fmt.Errorf("the ValidateBool differs")
	}
	if obj.ValidateError != res.ValidateError {
		return fmt.Errorf("the ValidateError differs")
	}
	if obj.AlwaysGroup != res.AlwaysGroup {
		return fmt.Errorf("the AlwaysGroup differs")
	}
	if obj.SendValue != res.SendValue {
		return fmt.Errorf("the SendValue differs")
	}

	if obj.Comment != res.Comment {
		return fmt.Errorf("the Comment differs")
	}

	return nil
}

// TestUID is the UID struct for TestRes.
type TestUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *TestRes) UIDs() []engine.ResUID {
	x := &TestUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *TestRes) GroupCmp(r engine.GroupableRes) error {
	_, ok := r.(*TestRes)
	if !ok {
		return fmt.Errorf("resource is not the same kind")
	}
	if !obj.AlwaysGroup { // grouped together if we were asked to
		return fmt.Errorf("the AlwaysGroup param is false")
	}

	return nil
}

// TestSends is the struct of data which is sent after a successful Apply.
type TestSends struct {
	// Hello is some value being sent.
	Hello  *string `lang:"hello"`
	Answer int     `lang:"answer"` // some other value being sent
}

// Sends represents the default struct of values we can send using Send/Recv.
func (obj *TestRes) Sends() interface{} {
	return &TestSends{
		Hello:  nil,
		Answer: -1,
	}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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

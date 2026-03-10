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

package resources

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
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

	//Uintptr uintptr `lang:"uintptr" yaml:"uintptr"`

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

	// Int8PtrPtrPtr probably makes no sense, but is legal.
	Int8PtrPtrPtr ***int8 `lang:"int8ptrptrptr" yaml:"int8ptrptrptr"`

	SliceString []string          `lang:"slicestring" yaml:"slicestring"`
	MapIntFloat map[int64]float64 `lang:"mapintfloat" yaml:"mapintfloat"`
	MixedStruct struct {
		SomeBool         bool    `lang:"somebool" yaml:"somebool"`
		SomeStr          string  `lang:"somestr" yaml:"somestr"`
		SomeInt          int64   `lang:"someint" yaml:"someint"`
		SomeFloat        float64 `lang:"somefloat" yaml:"somefloat"`
		somePrivatefield string
	} `lang:"mixedstruct" yaml:"mixedstruct"`
	Interface interface{} `lang:"interface" yaml:"interface"`

	AnotherStr string `lang:"anotherstr" yaml:"anotherstr"`

	// Func1 passes the value 42 to the input and returns a string.
	Func1 func(int) string `lang:"func1" yaml:"func1"`

	ValidateBool  bool      `lang:"validatebool" yaml:"validate_bool"`   // set to true to cause a validate error
	ValidateError string    `lang:"validateerror" yaml:"validate_error"` // set to cause a validate error
	AlwaysGroup   bool      `lang:"alwaysgroup" yaml:"always_group"`     // set to true to cause auto grouping
	CompareFail   bool      `lang:"comparefail" yaml:"compare_fail"`     // will compare fail?
	SendValue     string    `lang:"sendvalue" yaml:"send_value"`         // what value should we send?
	ExpectRecv    *[]string `lang:"expectrecv" yaml:"expect_recv"`       // what keys should we expect from send/recv?
	OnlyShow      []string  `lang:"onlyshow" yaml:"only_show"`           // what values do we show?

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

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *TestRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *TestRes) Watch(ctx context.Context) error {
	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	return nil
}

// CheckApply method for Test resource. Does nothing, returns happy!
func (obj *TestRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	expectRecv := []string{}
	for key, val := range obj.init.Recv() {
		obj.init.Logf("received `%s`, changed: %t", key, val.Changed)
		expectRecv = append(expectRecv, key)
	}

	if obj.init.Refresh() {
		obj.init.Logf("Received a notification!")
	}

	if obj.ExpectRecv != nil && len(*obj.ExpectRecv) != len(expectRecv) {
		return false, fmt.Errorf("the received keys differ from expected, got: %+v", expectRecv)
	}

	fakeLogf := func(format string, v ...interface{}) {
		key := format[0:strings.LastIndex(format, ":")]
		if len(obj.OnlyShow) == 0 || util.StrInList(key, obj.OnlyShow) {
			obj.init.Logf(format, v...)
		}
	}

	fakeLogf("Bool:          %v", obj.Bool)
	fakeLogf("Str:           %v", obj.Str)

	fakeLogf("Int:           %v", obj.Int)
	fakeLogf("Int8:          %v", obj.Int8)
	fakeLogf("Int16:         %v", obj.Int16)
	fakeLogf("Int32:         %v", obj.Int32)
	fakeLogf("Int64:         %v", obj.Int64)

	fakeLogf("Uint:          %v", obj.Uint)
	fakeLogf("Uint8:         %v", obj.Uint)
	fakeLogf("Uint16:        %v", obj.Uint)
	fakeLogf("Uint32:        %v", obj.Uint)
	fakeLogf("Uint64:        %v", obj.Uint)

	//fakeLogf("Uintptr:       %v", obj.Uintptr)
	fakeLogf("Byte:          %v", obj.Byte)
	fakeLogf("Rune:          %v", obj.Rune)

	fakeLogf("Float32:       %v", obj.Float32)
	fakeLogf("Float64:       %v", obj.Float64)
	fakeLogf("Complex64:     %v", obj.Complex64)
	fakeLogf("Complex128:    %v", obj.Complex128)

	fakeLogf("BoolPtr:       %v", obj.BoolPtr)
	fakeLogf("StringPtr:     %v", obj.StringPtr)
	fakeLogf("Int64Ptr:      %v", obj.Int64Ptr)
	fakeLogf("Int8Ptr:       %v", obj.Int8Ptr)
	fakeLogf("Uint8Ptr:      %v", obj.Uint8Ptr)

	fakeLogf("Int8PtrPtrPtr: %v", obj.Int8PtrPtrPtr)

	fakeLogf("SliceString:   %v", obj.SliceString)
	fakeLogf("MapIntFloat:   %v", obj.MapIntFloat)
	fakeLogf("MixedStruct:   %v", obj.MixedStruct)
	fakeLogf("Interface:     %v", obj.Interface)

	fakeLogf("AnotherStr:    %v", obj.AnotherStr)

	if obj.Func1 != nil {
		fakeLogf("Func1:         %v", obj.Func1(42))
	}

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
	if (obj.ExpectRecv == nil) != (res.ExpectRecv == nil) { // xor
		return fmt.Errorf("the ExpectRecv differs")
	}
	if obj.ExpectRecv != nil && res.ExpectRecv != nil {
		if len(*obj.ExpectRecv) != len(*res.ExpectRecv) {
			return fmt.Errorf("the length of ExpectRecv differs")
		}
		for i, x := range *obj.ExpectRecv {
			if x != (*res.ExpectRecv)[i] {
				return fmt.Errorf("the item at ExpectRecv index %d differs", i)
			}
		}
	}
	if len(obj.OnlyShow) != len(res.OnlyShow) {
		return fmt.Errorf("the length of OnlyShow differs")
	}
	for i, x := range obj.OnlyShow {
		if x != res.OnlyShow[i] {
			return fmt.Errorf("the item at OnlyShow index %d differs", i)
		}
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
	Hello  *string `lang:"hello" yaml:"hello"`
	Answer int     `lang:"answer" yaml:"answer"` // some other value being sent
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

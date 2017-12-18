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
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"os/user"
	"reflect"
	"strconv"
	"testing"
)

func TestSort0(t *testing.T) {
	rs := []Res{}
	s := Sort(rs)

	if !reflect.DeepEqual(s, []Res{}) {
		t.Errorf("sort failed!")
		if s == nil {
			t.Logf("output is nil!")
		} else {
			str := "Got:"
			for _, r := range s {
				str += " " + r.String()
			}
			t.Errorf(str)
		}
	}
}

func TestSort1(t *testing.T) {
	r1, _ := NewNamedResource("noop", "noop1")
	r2, _ := NewNamedResource("noop", "noop2")
	r3, _ := NewNamedResource("noop", "noop3")
	r4, _ := NewNamedResource("noop", "noop4")
	r5, _ := NewNamedResource("noop", "noop5")
	r6, _ := NewNamedResource("noop", "noop6")

	rs := []Res{r3, r2, r6, r1, r5, r4}
	s := Sort(rs)

	if !reflect.DeepEqual(s, []Res{r1, r2, r3, r4, r5, r6}) {
		t.Errorf("sort failed!")
		str := "Got:"
		for _, r := range s {
			str += " " + r.String()
		}
		t.Errorf(str)
	}

	if !reflect.DeepEqual(rs, []Res{r3, r2, r6, r1, r5, r4}) {
		t.Errorf("sort modified input!")
		str := "Got:"
		for _, r := range rs {
			str += " " + r.String()
		}
		t.Errorf(str)
	}
}

func TestMiscEncodeDecode1(t *testing.T) {
	var err error

	// encode
	var input interface{} = &FileRes{}
	b1 := bytes.Buffer{}
	e := gob.NewEncoder(&b1)
	err = e.Encode(&input) // pass with &
	if err != nil {
		t.Errorf("Gob failed to Encode: %v", err)
	}
	str := base64.StdEncoding.EncodeToString(b1.Bytes())

	// decode
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		t.Errorf("Base64 failed to Decode: %v", err)
	}
	b2 := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b2)
	err = d.Decode(&output) // pass with &
	if err != nil {
		t.Errorf("Gob failed to Decode: %v", err)
	}

	res1, ok := input.(Res)
	if !ok {
		t.Errorf("Input %v is not a Res", res1)
		return
	}
	res2, ok := output.(Res)
	if !ok {
		t.Errorf("Output %v is not a Res", res2)
		return
	}
	if !res1.Compare(res2) {
		t.Error("The input and output Res values do not match!")
	}
}

func TestMiscEncodeDecode2(t *testing.T) {
	var err error

	// encode
	input, err := NewNamedResource("file", "file1")
	if err != nil {
		t.Errorf("Can't create: %v", err)
		return
	}

	b64, err := ResToB64(input)
	if err != nil {
		t.Errorf("Can't encode: %v", err)
		return
	}

	output, err := B64ToRes(b64)
	if err != nil {
		t.Errorf("Can't decode: %v", err)
		return
	}

	res1, ok := input.(Res)
	if !ok {
		t.Errorf("Input %v is not a Res", res1)
		return
	}
	res2, ok := output.(Res)
	if !ok {
		t.Errorf("Output %v is not a Res", res2)
		return
	}
	if !res1.Compare(res2) {
		t.Error("The input and output Res values do not match!")
	}
}

func TestStructTagToFieldName0(t *testing.T) {
	type TestStruct struct {
		TestRes        // so that this struct implements `Res`
		Alpha   bool   `lang:"alpha" yaml:"nope"`
		Beta    string `yaml:"beta"`
		Gamma   string
		Delta   int `lang:"surprise"`
	}

	mapping, err := StructTagToFieldName(&TestStruct{})
	if err != nil {
		t.Errorf("failed: %+v", err)
		return
	}

	expected := map[string]string{
		"alpha":    "Alpha",
		"surprise": "Delta",
	}

	if !reflect.DeepEqual(mapping, expected) {
		t.Errorf("expected: %+v", expected)
		t.Errorf("received: %+v", mapping)
	}
}

func TestLowerStructFieldNameToFieldName0(t *testing.T) {
	type TestStruct struct {
		TestRes   // so that this struct implements `Res`
		Alpha     bool
		skipMe    bool
		Beta      string
		IAmACamel uint
		pass      *string
		Gamma     string
		Delta     int
	}

	mapping, err := LowerStructFieldNameToFieldName(&TestStruct{})
	if err != nil {
		t.Errorf("failed: %+v", err)
		return
	}

	expected := map[string]string{
		"testres": "TestRes", // hide by specifying `lang:""` on it
		"alpha":   "Alpha",
		//"skipme": "skipMe",
		"beta":      "Beta",
		"iamacamel": "IAmACamel",
		//"pass": "pass",
		"gamma": "Gamma",
		"delta": "Delta",
	}

	if !reflect.DeepEqual(mapping, expected) {
		t.Errorf("expected: %+v", expected)
		t.Errorf("received: %+v", mapping)
	}
}

func TestLowerStructFieldNameToFieldName1(t *testing.T) {
	type TestStruct struct {
		TestRes // so that this struct implements `Res`
		Alpha   bool
		skipMe  bool
		Beta    string
		// these two should collide
		DoubleWord bool
		Doubleword string
		IAmACamel  uint
		pass       *string
		Gamma      string
		Delta      int
	}

	mapping, err := LowerStructFieldNameToFieldName(&TestStruct{})
	if err == nil {
		t.Errorf("expected failure, but passed with: %+v", mapping)
		return
	}
}

func TestLowerStructFieldNameToFieldName2(t *testing.T) {
	mapping, err := LowerStructFieldNameToFieldName(&TestRes{})
	if err != nil {
		t.Errorf("failed: %+v", err)
		return
	}

	expected := map[string]string{
		"baseres": "BaseRes",

		"bool": "Bool",
		"str":  "Str",

		"int":   "Int",
		"int8":  "Int8",
		"int16": "Int16",
		"int32": "Int32",
		"int64": "Int64",

		"uint":   "Uint",
		"uint8":  "Uint8",
		"uint16": "Uint16",
		"uint32": "Uint32",
		"uint64": "Uint64",

		"byte": "Byte",
		"rune": "Rune",

		"float32":    "Float32",
		"float64":    "Float64",
		"complex64":  "Complex64",
		"complex128": "Complex128",

		"boolptr":   "BoolPtr",
		"stringptr": "StringPtr",
		"int64ptr":  "Int64Ptr",
		"int8ptr":   "Int8Ptr",
		"uint8ptr":  "Uint8Ptr",

		"int8ptrptrptr": "Int8PtrPtrPtr",

		"slicestring": "SliceString",
		"mapintfloat": "MapIntFloat",
		"mixedstruct": "MixedStruct",
		"interface":   "Interface",

		"anotherstr": "AnotherStr",

		"validatebool":  "ValidateBool",
		"validateerror": "ValidateError",
		"alwaysgroup":   "AlwaysGroup",
		"comparefail":   "CompareFail",

		"comment": "Comment",
	}

	if !reflect.DeepEqual(mapping, expected) {
		t.Errorf("expected: %+v", expected)
		t.Errorf("received: %+v", mapping)
	}
}

func TestUnknownGroup(t *testing.T) {
	gid, err := GetGID("unknowngroup")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", gid)
	}
}

func TestUnknownUser(t *testing.T) {
	uid, err := GetUID("unknownuser")
	if err == nil {
		t.Errorf("expected failure, but passed with: %d", uid)
	}
}

func TestCurrentUserGroupByName(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via username and group name)
	if uid, err = GetUID(userObj.Username); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	// macOS users do not have a group with their name on it, so not assuming this here
	group, err := user.LookupGroupId(currentGID)
	if err != nil {
		t.Errorf("failed to lookup group by id: %s", currentGID)
	}
	if gid, err = GetGID(group.Name); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}

func TestCurrentUserGroupById(t *testing.T) {
	// get current user
	userObj, err := user.Current()
	if err != nil {
		t.Errorf("error trying to lookup current user: %s", err.Error())
	}

	currentUID := userObj.Uid
	currentGID := userObj.Gid

	var uid int
	var gid int

	// now try to get the uid/gid via our API (via uid and gid)
	if uid, err = GetUID(currentUID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(uid) != currentUID {
		t.Errorf("uid didn't match current user's: %s vs %s", strconv.Itoa(uid), currentUID)
	}

	if gid, err = GetGID(currentGID); err != nil {
		t.Errorf("error trying to lookup current user UID: %s", err.Error())
	}

	if strconv.Itoa(gid) != currentGID {
		t.Errorf("gid didn't match current user's: %s vs %s", strconv.Itoa(gid), currentGID)
	}
}

func TestStrSortedSliceCompare0(t *testing.T) {
	slice1 := []string{"foo", "bar", "baz"}
	slice2 := []string{"bar", "foo", "baz"}

	if err := StrSortedSliceCompare(slice1, slice2); err != nil {
		t.Errorf("slices were not evaluated as equivalent: %v, %v", slice1, slice2)
	}
}

func TestStrSortedSliceCompare1(t *testing.T) {
	slice1 := []string{"foo", "bar", "baz"}
	slice2 := []string{"fi", "fi", "fo"}

	if err := StrSortedSliceCompare(slice1, slice2); err == nil {
		t.Errorf("slices were evaluated as equivalent: %v, %v", slice1, slice2)
	}
}

func TestStrSortedSliceCompare2(t *testing.T) {
	slice1 := []string{"foo", "bar", "baz"}
	slice2 := []string{"foo", "bar"}

	if err := StrSortedSliceCompare(slice1, slice2); err == nil {
		t.Errorf("slices were evaluated as equivalent: %v, %v", slice1, slice2)
	}
}

func TestStrSortedSliceCompare3(t *testing.T) {
	slice0 := []string{"foo", "bar", "baz"}
	slice1 := []string{"zip", "zap", "zop"}

	_ = StrSortedSliceCompare(slice0, slice1)

	if slice0[0] != "foo" || slice0[1] != "bar" || slice0[2] != "baz" {
		t.Errorf("input slice reordered to: %v", slice0)
	}

	if slice1[0] != "zip" || slice1[1] != "zap" || slice1[2] != "zop" {
		t.Errorf("input slice reordered to: %v", slice1)
	}
}

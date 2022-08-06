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

//go:build !root

package resources

import (
	"reflect"
	"testing"

	engineUtil "github.com/purpleidea/mgmt/engine/util"
)

func TestStructTagToFieldName0(t *testing.T) {
	type TestStruct struct {
		TestRes        // so that this struct implements `Res`
		Alpha   bool   `lang:"alpha" yaml:"nope"`
		Beta    string `yaml:"beta"`
		Gamma   string
		Delta   int `lang:"surprise"`
	}

	mapping, err := engineUtil.StructTagToFieldName(&TestStruct{})
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

	mapping, err := engineUtil.LowerStructFieldNameToFieldName(&TestStruct{})
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

	mapping, err := engineUtil.LowerStructFieldNameToFieldName(&TestStruct{})
	if err == nil {
		t.Errorf("expected failure, but passed with: %+v", mapping)
		return
	}
}

func TestLowerStructFieldNameToFieldName2(t *testing.T) {
	mapping, err := engineUtil.LowerStructFieldNameToFieldName(&TestRes{})
	if err != nil {
		t.Errorf("failed: %+v", err)
		return
	}

	expected := map[string]string{
		"base":        "Base",        // all resources have this trait
		"groupable":   "Groupable",   // the TestRes has this trait
		"refreshable": "Refreshable", // the TestRes has this trait
		"sendable":    "Sendable",
		"recvable":    "Recvable",

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
		"sendvalue":     "SendValue",

		"comment": "Comment",
	}

	if !reflect.DeepEqual(mapping, expected) {
		t.Errorf("expected: %+v", expected)
		t.Errorf("received: %+v", mapping)
	}
}

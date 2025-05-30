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

//go:build !root

package types

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/util"
)

func TestType0(t *testing.T) {
	str := "struct{a bool; bb int; ccc str}"
	val := &Type{
		Kind: KindStruct,
		Ord: []string{
			"a",
			"bb",
			"ccc",
		},
		Map: map[string]*Type{
			"a": {
				Kind: KindBool,
			},
			"bb": {
				Kind: KindInt,
			},
			"ccc": {
				Kind: KindStr,
			},
		},
	}
	kind := NewType(str)
	if err := kind.Cmp(val); err != nil {
		t.Errorf("kind output of `%v` did not match expected: `%v`", str, err)
	}
}

func TestType1(t *testing.T) {
	testCases := map[string]*Type{
		"":     nil, // error
		"nope": nil, // error

		// basic types
		"bool": {
			Kind: KindBool,
		},
		"str": {
			Kind: KindStr,
		},
		"int": {
			Kind: KindInt,
		},
		"float": {
			Kind: KindFloat,
		},

		// lists
		"[]str": { // list of str's
			Kind: KindList,
			Val: &Type{
				Kind: KindStr,
			},
		},
		"[]int": {
			Kind: KindList,
			Val: &Type{
				Kind: KindInt,
			},
		},
		"[]bool": {
			Kind: KindList,
			Val: &Type{
				Kind: KindBool,
			},
		},

		// nested lists
		"[][]bool": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindBool,
				},
			},
		},
		"[][]int": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindInt,
				},
			},
		},
		"[][][]str": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindList,
					Val: &Type{
						Kind: KindStr,
					},
				},
			},
		},

		// maps
		"map{}": nil, // invalid
		"map{str: str}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindStr,
			},
		},
		"map{str: int}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindInt,
			},
		},
		"map{str: variant}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindVariant,
			},
		},
		"map{variant: int}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindVariant,
			},
			Val: &Type{
				Kind: KindInt,
			},
		},
		"map{variant: variant}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindVariant,
			},
			Val: &Type{
				Kind: KindVariant,
			},
		},

		// nested maps
		"map{str: map{int: bool}}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindBool,
				},
			},
		},
		"map{map{int: bool}: str}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindBool,
				},
			},
			Val: &Type{
				Kind: KindStr,
			},
		},
		"map{map{str: int}: map{int: bool}}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindStr,
				},
				Val: &Type{
					Kind: KindInt,
				},
			},
			Val: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindBool,
				},
			},
		},
		"map{str: map{int: map{int: bool}}}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindMap,
					Key: &Type{
						Kind: KindInt,
					},
					Val: &Type{
						Kind: KindBool,
					},
				},
			},
		},

		// structs
		"struct{}": {
			Kind: KindStruct,
			Map:  map[string]*Type{},
		},
		"struct{a bool}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindBool,
				},
			},
		},
		"struct{a bool; bb int}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
				"bb",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindBool,
				},
				"bb": {
					Kind: KindInt,
				},
			},
		},
		"struct{a bool; bb int; ccc str}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
				"bb",
				"ccc",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindBool,
				},
				"bb": {
					Kind: KindInt,
				},
				"ccc": {
					Kind: KindStr,
				},
			},
		},

		// nested structs
		"struct{bb struct{z bool}; ccc str}": {
			Kind: KindStruct,
			Ord: []string{
				"bb",
				"ccc",
			},
			Map: map[string]*Type{
				"bb": {
					Kind: KindStruct,
					Ord: []string{
						"z",
					},
					Map: map[string]*Type{
						"z": {
							Kind: KindBool,
						},
					},
				},
				"ccc": {
					Kind: KindStr,
				},
			},
		},
		"struct{a bool; bb struct{z bool; yy int}; ccc str}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
				"bb",
				"ccc",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindBool,
				},
				"bb": {
					Kind: KindStruct,
					Ord: []string{
						"z",
						"yy",
					},
					Map: map[string]*Type{
						"z": {
							Kind: KindBool,
						},
						"yy": {
							Kind: KindInt,
						},
					},
				},
				"ccc": {
					Kind: KindStr,
				},
			},
		},
		"struct{a bool; bb struct{z bool; yy struct{struct int; nested bool}}; ccc str}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
				"bb",
				"ccc",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindBool,
				},
				"bb": {
					Kind: KindStruct,
					Ord: []string{
						"z",
						"yy",
					},
					Map: map[string]*Type{
						"z": {
							Kind: KindBool,
						},
						"yy": {
							Kind: KindStruct,
							Ord: []string{
								"struct",
								"nested",
							},
							Map: map[string]*Type{
								"struct": {
									Kind: KindInt,
								},
								"nested": {
									Kind: KindBool,
								},
							},
						},
					},
				},
				"ccc": {
					Kind: KindStr,
				},
			},
		},

		// mixed nesting
		"map{str: []struct{a bool; int []bool}}": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindStruct,
					Ord: []string{
						"a",
						"int",
					},
					Map: map[string]*Type{
						"a": {
							Kind: KindBool,
						},
						"int": {
							Kind: KindList,
							Val: &Type{
								Kind: KindBool,
							},
						},
					},
				},
			},
		},
		"struct{a map{str: map{struct{deeply int; nested bool}: map{int: bool}}}; bb struct{z bool; yy int}; ccc str}": {
			Kind: KindStruct,
			Ord: []string{
				"a",
				"bb",
				"ccc",
			},
			Map: map[string]*Type{
				"a": {
					Kind: KindMap,
					Key: &Type{
						Kind: KindStr,
					},
					Val: &Type{
						Kind: KindMap,
						Key: &Type{
							Kind: KindStruct,
							Ord: []string{
								"deeply",
								"nested",
							},
							Map: map[string]*Type{
								"deeply": {
									Kind: KindInt,
								},
								"nested": {
									Kind: KindBool,
								},
							},
						},
						Val: &Type{
							Kind: KindMap,
							Key: &Type{
								Kind: KindInt,
							},
							Val: &Type{
								Kind: KindBool,
							},
						},
					},
				},
				"bb": {
					Kind: KindStruct,
					Ord: []string{
						"z",
						"yy",
					},
					Map: map[string]*Type{
						"z": {
							Kind: KindBool,
						},
						"yy": {
							Kind: KindInt,
						},
					},
				},
				"ccc": {
					Kind: KindStr,
				},
			},
		},

		// functions
		"func()": {
			Kind: KindFunc,
			Map:  map[string]*Type{},
			Ord:  []string{},
			Out:  nil,
		},
		"func() float": {
			Kind: KindFunc,
			Map:  map[string]*Type{},
			Ord:  []string{},
			Out: &Type{
				Kind: KindFloat,
			},
		},
		"func(a0 str) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"a0": {
					Kind: KindStr,
				},
			},
			Ord: []string{
				"a0", // must match
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(hello str, answer int) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"hello": {
					Kind: KindStr,
				},
				"answer": {
					Kind: KindInt,
				},
			},
			Ord: []string{
				"hello",
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(a0 str, a1 []int, a2 float) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"a0": {
					Kind: KindStr,
				},
				"a1": {
					Kind: KindList,
					Val: &Type{
						Kind: KindInt,
					},
				},
				"a2": {
					Kind: KindFloat,
				},
			},
			Ord: []string{
				"a0",
				"a1",
				"a2",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(answer map{str: int}) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"answer": {
					Kind: KindMap,
					Key: &Type{
						Kind: KindStr,
					},
					Val: &Type{
						Kind: KindInt,
					},
				},
			},
			Ord: []string{
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(hello bool, answer map{str: int}) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"hello": {
					Kind: KindBool,
				},
				"answer": {
					Kind: KindMap,
					Key: &Type{
						Kind: KindStr,
					},
					Val: &Type{
						Kind: KindInt,
					},
				},
			},
			Ord: []string{
				"hello",
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(answer struct{a str; bb int}) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"answer": {
					Kind: KindStruct,
					Ord: []string{
						"a",
						"bb",
					},
					Map: map[string]*Type{
						"a": {
							Kind: KindStr,
						},
						"bb": {
							Kind: KindInt,
						},
					},
				},
			},
			Ord: []string{
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(hello bool, answer struct{a str; bb int}) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"hello": {
					Kind: KindBool,
				},
				"answer": {
					Kind: KindStruct,
					Ord: []string{
						"a",
						"bb",
					},
					Map: map[string]*Type{
						"a": {
							Kind: KindStr,
						},
						"bb": {
							Kind: KindInt,
						},
					},
				},
			},
			Ord: []string{
				"hello",
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
	}

	for str, val := range testCases { // run all the tests

		// for debugging
		//if str != "func(str, int) bool" {
		//	continue
		//}

		// check the type
		typ := NewType(str)
		//t.Logf("str: %+v", str)
		//t.Logf("typ: %+v", typ)
		//if !reflect.DeepEqual(kind, val) {
		//	t.Errorf("kind output of `%v` did not match expected: `%v`", kind, val)
		//}

		if val == nil { // catch error cases
			if typ != nil {
				t.Errorf("invalid type: `%s` did not match expected nil", str)
			}
			continue
		}

		if err := typ.Cmp(val); err != nil {
			t.Errorf("type: `%s` did not match expected: `%v`", str, err)
			return
		}

		// check the string
		if repr := val.String(); repr != str {
			t.Errorf("type representation of `%s` did not match expected: `%s`", str, repr)
		}
	}
}

func TestType2(t *testing.T) {
	// mapping from golang representation to our expected equivalent
	testCases := map[string]*Type{
		// basic types
		"bool": {
			Kind: KindBool,
		},
		"string": {
			Kind: KindStr,
		},
		"int64": {
			Kind: KindInt,
		},
		"float64": {
			Kind: KindFloat,
		},

		// lists
		"[]bool": {
			Kind: KindList,
			Val: &Type{
				Kind: KindBool,
			},
		},
		"[]string": { // list of str's
			Kind: KindList,
			Val: &Type{
				Kind: KindStr,
			},
		},
		"[]int64": {
			Kind: KindList,
			Val: &Type{
				Kind: KindInt,
			},
		},

		// nested lists
		"[][]bool": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindBool,
				},
			},
		},
		"[][]int64": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindInt,
				},
			},
		},
		"[][][]string": {
			Kind: KindList,
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindList,
					Val: &Type{
						Kind: KindStr,
					},
				},
			},
		},

		// maps
		"map[string]string": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindStr,
			},
		},
		"map[string]int64": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindInt,
			},
		},

		// nested maps
		"map[string]map[int64]bool": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindBool,
				},
			},
		},
		// FIXME: should we prevent this in our implementation as well?
		//"map[map[int64]bool]string": &Type{ // no map keys in golang!
		//	Kind: KindMap,
		//	Key: &Type{
		//		Kind: KindMap,
		//		Key: &Type{
		//			Kind: KindInt,
		//		},
		//		Val: &Type{
		//			Kind: KindBool,
		//		},
		//	},
		//	Val: &Type{
		//		Kind: KindStr,
		//	},
		//},
		//"map[map[string]int64]map[int64]bool": &Type{
		//	Kind: KindMap,
		//	Key: &Type{
		//		Kind: KindMap,
		//		Key: &Type{
		//			Kind: KindStr,
		//		},
		//		Val: &Type{
		//			Kind: KindInt,
		//		},
		//	},
		//	Val: &Type{
		//		Kind: KindMap,
		//		Key: &Type{
		//			Kind: KindInt,
		//		},
		//		Val: &Type{
		//			Kind: KindBool,
		//		},
		//	},
		//},
		"map[string]map[int64]map[int64]bool": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindInt,
				},
				Val: &Type{
					Kind: KindMap,
					Key: &Type{
						Kind: KindInt,
					},
					Val: &Type{
						Kind: KindBool,
					},
				},
			},
		},

		// structs
		"struct {}": { // requires a space between `struct` and {}
			Kind: KindStruct,
			Map:  map[string]*Type{},
		},
		"struct { A bool }": { // more spaces, and uppercase keys!
			Kind: KindStruct,
			Ord: []string{
				"A",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindBool,
				},
			},
		},
		"struct { A bool; Bb int64 }": {
			Kind: KindStruct,
			Ord: []string{
				"A",
				"Bb",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindBool,
				},
				"Bb": {
					Kind: KindInt,
				},
			},
		},
		"struct { A bool; Bb int64; Ccc string }": {
			Kind: KindStruct,
			Ord: []string{
				"A",
				"Bb",
				"Ccc",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindBool,
				},
				"Bb": {
					Kind: KindInt,
				},
				"Ccc": {
					Kind: KindStr,
				},
			},
		},

		// nested structs
		"struct { Bb struct { Z bool }; Ccc string }": {
			Kind: KindStruct,
			Ord: []string{
				"Bb",
				"Ccc",
			},
			Map: map[string]*Type{
				"Bb": {
					Kind: KindStruct,
					Ord: []string{
						"Z",
					},
					Map: map[string]*Type{
						"Z": {
							Kind: KindBool,
						},
					},
				},
				"Ccc": {
					Kind: KindStr,
				},
			},
		},
		"struct { A bool; Bb struct { Z bool; Yy int64 }; Ccc string }": {
			Kind: KindStruct,
			Ord: []string{
				"A",
				"Bb",
				"Ccc",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindBool,
				},
				"Bb": {
					Kind: KindStruct,
					Ord: []string{
						"Z",
						"Yy",
					},
					Map: map[string]*Type{
						"Z": {
							Kind: KindBool,
						},
						"Yy": {
							Kind: KindInt,
						},
					},
				},
				"Ccc": {
					Kind: KindStr,
				},
			},
		},
		"struct { A bool; Bb struct { Z bool; Yy struct { Struct int64; Nested bool } }; Ccc string }": {
			Kind: KindStruct,
			Ord: []string{
				"A",
				"Bb",
				"Ccc",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindBool,
				},
				"Bb": {
					Kind: KindStruct,
					Ord: []string{
						"Z",
						"Yy",
					},
					Map: map[string]*Type{
						"Z": {
							Kind: KindBool,
						},
						"Yy": {
							Kind: KindStruct,
							Ord: []string{
								"Struct",
								"Nested",
							},
							Map: map[string]*Type{
								"Struct": {
									Kind: KindInt,
								},
								"Nested": {
									Kind: KindBool,
								},
							},
						},
					},
				},
				"Ccc": {
					Kind: KindStr,
				},
			},
		},

		// mixed nesting
		"map[string][]struct { A bool; Int64 []bool }": {
			Kind: KindMap,
			Key: &Type{
				Kind: KindStr,
			},
			Val: &Type{
				Kind: KindList,
				Val: &Type{
					Kind: KindStruct,
					Ord: []string{
						"A",
						"Int64",
					},
					Map: map[string]*Type{
						"A": {
							Kind: KindBool,
						},
						"Int64": {
							Kind: KindList,
							Val: &Type{
								Kind: KindBool,
							},
						},
					},
				},
			},
		},

		"struct { A map[string]map[struct { Deeply int64; Nested bool }]map[int64]bool; Bb struct { Z bool; Yy int64 }; Ccc string }": {
			Kind: KindStruct,
			Ord: []string{
				"A",
				"Bb",
				"Ccc",
			},
			Map: map[string]*Type{
				"A": {
					Kind: KindMap,
					Key: &Type{
						Kind: KindStr,
					},
					Val: &Type{
						Kind: KindMap,
						Key: &Type{
							Kind: KindStruct,
							Ord: []string{
								"Deeply",
								"Nested",
							},
							Map: map[string]*Type{
								"Deeply": {
									Kind: KindInt,
								},
								"Nested": {
									Kind: KindBool,
								},
							},
						},
						Val: &Type{
							Kind: KindMap,
							Key: &Type{
								Kind: KindInt,
							},
							Val: &Type{
								Kind: KindBool,
							},
						},
					},
				},
				"Bb": {
					Kind: KindStruct,
					Ord: []string{
						"Z",
						"Yy",
					},
					Map: map[string]*Type{
						"Z": {
							Kind: KindBool,
						},
						"Yy": {
							Kind: KindInt,
						},
					},
				},
				"Ccc": {
					Kind: KindStr,
				},
			},
		},

		// functions
		"func()": {
			Kind: KindFunc,
			Map:  map[string]*Type{},
			Ord:  []string{},
			Out:  nil,
		},
		"func() float64": {
			Kind: KindFunc,
			Map:  map[string]*Type{},
			Ord:  []string{},
			Out: &Type{
				Kind: KindFloat,
			},
		},
		"func(string) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"a0": {
					Kind: KindStr,
				},
			},
			Ord: []string{
				"a0", // must match
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(string, int64) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"hello": {
					Kind: KindStr,
				},
				"answer": {
					Kind: KindInt,
				},
			},
			Ord: []string{
				"hello",
				"answer",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(string, []int64, float64) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"a0": {
					Kind: KindStr,
				},
				"a1": {
					Kind: KindList,
					Val: &Type{
						Kind: KindInt,
					},
				},
				"a2": {
					Kind: KindFloat,
				},
			},
			Ord: []string{
				"a0",
				"a1",
				"a2",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
	}

	for str, typ := range testCases { // run all the tests
		// check the type
		reflected := typ.Reflect()

		//t.Logf("reflect: %+v -> %+v", str, reflected.String())
		// check the string
		if repr := reflected.String(); repr != str {
			t.Errorf("type representation of `%s` did not match expected: `%s`", str, repr)
		}
	}
}

func TestType3(t *testing.T) {
	// functions with named types...
	testCases := map[string]*Type{
		"func(input str) bool": {
			Kind: KindFunc,
			Map: map[string]*Type{
				"input": {
					Kind: KindStr,
				},
			},
			Ord: []string{
				"input", // must match
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(a str) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"a": {
					Kind: KindStr,
				},
			},
			Ord: []string{
				"a", // must match
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(aaa str, bb int) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"aaa": {
					Kind: KindStr,
				},
				"bb": {
					Kind: KindInt,
				},
			},
			Ord: []string{
				"aaa",
				"bb",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
		"func(aaa map{str: int}) bool": {
			Kind: KindFunc,
			// key names are arbitrary...
			Map: map[string]*Type{
				"aaa": {
					Kind: KindMap,
					Key: &Type{
						Kind: KindStr,
					},
					Val: &Type{
						Kind: KindInt,
					},
				},
			},
			Ord: []string{
				"aaa",
			},
			Out: &Type{
				Kind: KindBool,
			},
		},
	}

	for str, val := range testCases { // run all the tests

		// for debugging
		//if str != "func(aaa str, bb int) bool" {
		//continue
		//}

		// check the type
		typ := NewType(str)
		//t.Logf("str: %+v", str)
		//t.Logf("typ: %+v", typ)
		//if !reflect.DeepEqual(kind, val) {
		//	t.Errorf("kind output of `%v` did not match expected: `%v`", kind, val)
		//}

		if val == nil { // catch error cases
			if typ != nil {
				t.Errorf("invalid type: `%s` did not match expected nil", str)
			}
			continue
		}

		if err := typ.Cmp(val); err != nil {
			t.Errorf("type: `%s` did not match expected: `%v`", str, err)
			return
		}
	}
}

func TestComplexCmp0(t *testing.T) {
	type test struct { // an individual test
		name string
		typ1 *Type
		typ2 *Type
		err  bool   // expected err ?
		str  string // expected output str
	}
	testCases := []test{}

	{
		testCases = append(testCases, test{
			name: "int vs str",
			typ1: TypeInt,
			typ2: TypeStr,
			err:  true,
			str:  "",
		})
	}
	{
		testCases = append(testCases, test{
			name: "nested list vs list variant",
			typ1: NewType("[][]str"),
			typ2: &Type{
				Kind: KindList,
				Val:  TypeVariant,
			},
			err: false,
			str: "variant",
		})
	}
	{
		testCases = append(testCases, test{
			name: "nil vs type",
			typ1: nil,
			typ2: NewType("[][]str"),
			err:  false,
			str:  "partial",
		})
	}
	{
		testCases = append(testCases, test{
			name: "variant vs type",
			typ1: TypeVariant,
			typ2: NewType("[][]str"),
			err:  false,
			str:  "variant",
		})
	}
	{
		testCases = append(testCases, test{
			name: "nil vs variant",
			typ1: nil,
			typ2: TypeVariant,
			err:  false,
			str:  "both",
		})
	}
	{
		testCases = append(testCases, test{
			name: "type vs nil",
			typ1: NewType("[][]str"),
			typ2: nil,
			err:  false,
			str:  "partial",
		})
	}
	{
		testCases = append(testCases, test{
			name: "type vs variant",
			typ1: NewType("[][]str"),
			typ2: TypeVariant,
			err:  false,
			str:  "variant",
		})
	}
	{
		testCases = append(testCases, test{
			name: "variant vs nil",
			typ1: TypeVariant,
			typ2: nil,
			err:  false,
			str:  "both",
		})
	}
	{
		// func([]int) VS func([]variant) int
		testCases = append(testCases, test{
			name: "partial vs variant",
			typ1: &Type{
				Kind: KindFunc,
				Map: map[string]*Type{
					"ints": {
						Kind: KindList,
						Val:  TypeInt,
					},
				},
				Ord: []string{"ints"},
				Out: nil, // unspecified, it's a partial
			},
			typ2: &Type{
				Kind: KindFunc,
				Map: map[string]*Type{
					"ints": {
						Kind: KindList,
						Val:  TypeVariant, // variant!
					},
				},
				Ord: []string{"ints"},
				Out: TypeInt,
			},
			err: false,
			str: "both",
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			typ1, typ2, err, str := tc.typ1, tc.typ2, tc.err, tc.str

			// the reverse should probably match the forward version
			s1, err1 := typ1.ComplexCmp(typ2)
			s2, err2 := typ2.ComplexCmp(typ1)

			if err && err1 == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected error, got nil", index)
			}
			if !err && err1 != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unexpected error: %+v", index, err1)
			}
			if err && err2 == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected error, got nil", index)
			}
			if !err && err2 != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unexpected error: %+v", index, err2)
			}

			if s1 != s2 {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: strings did not match: %+v != %+v", index, s1, s2)
				return
			}
			if s1 != str {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unexpected string: %+v != %+v", index, s1, str)
				return
			}
		})
	}
}

func TestTypeCopy0(t *testing.T) {
	typ := NewType("func(arg0 str, arg1 bool) int")
	cp := typ.Copy()
	if len(cp.Ord) != 2 {
		t.Errorf("incorrect ord length after Copy")
	}
	if cp.Ord[0] != "arg0" {
		t.Errorf("incorrect 0th arg name after Copy")
	}
	if cp.Ord[1] != "arg1" {
		t.Errorf("incorrect 1st arg name after Copy")
	}
}

func TestUni0(t *testing.T) {
	// good type strings
	if NewType("?1") == nil {
		t.Errorf("unexpected nil type")
	}
	if NewType("?123") == nil {
		t.Errorf("unexpected nil type")
	}
	if NewType("[]?123") == nil {
		t.Errorf("unexpected nil type")
	}
	if NewType("map{?123: ?123}") == nil {
		t.Errorf("unexpected nil type")
	}

	// bad type strings
	if typ := NewType("?0"); typ != nil {
		t.Errorf("expected nil type, got: %v", typ)
	}
	if typ := NewType("?00"); typ != nil {
		t.Errorf("expected nil type, got: %v", typ)
	}
	if typ := NewType("?000000000000000000000"); typ != nil {
		t.Errorf("expected nil type, got: %v", typ)
	}
}

func TestUni1(t *testing.T) {
	// functions with named types...
	testCases := map[string]*Type{
		// good type strings
		"?1": {
			Kind: KindUnification,
			Uni:  NewElem(),
		},
		"?123": {
			Kind: KindUnification,
			Uni:  NewElem(),
		},
		"[]?123": {
			Kind: KindList,
			Val: &Type{
				Kind: KindUnification,
				Uni:  NewElem(),
			},
		},

		// bad type strings
		"?0":     nil,
		"?00":    nil,
		"?00000": nil,
		"?-1":    nil,
		"?-42":   nil,
		"?0x42":  nil, // hexadecimal
		"?013":   nil, // octal
	}

	for str, val := range testCases { // run all the tests
		// for debugging
		//if str != "?0" {
		//continue
		//}

		// check the type
		typ := NewType(str)
		//t.Logf("str: %+v", str)
		//t.Logf("typ: %+v", typ)

		if val == nil { // catch error cases
			if typ != nil {
				t.Errorf("invalid type: `%s` did not match expected nil", str)
			}
			continue
		}

		if err := typ.Cmp(val); err != nil {
			t.Errorf("type: `%s` did not match expected: `%v`", str, err)
			return
		}
	}
}

func TestUniCmp0(t *testing.T) {
	type test struct { // an individual test
		name string
		typ1 *Type
		typ2 *Type
		err  bool   // expected err ?
		str  string // expected output str
	}
	testCases := []test{}

	testCases = append(testCases, test{
		name: "simple ?1 compare",
		typ1: NewType("?1"),
		typ2: NewType("?1"),
		err:  false,
	})
	testCases = append(testCases, test{
		name: "different ?1 compare",
		typ1: NewType("?13"), // they don't need to be the same
		typ2: NewType("?42"),
		err:  false,
	})
	testCases = append(testCases, test{
		name: "duplicate type unification variables",
		// the type unification variables should be the same
		typ1: NewType("map{?123:?123}"),
		typ2: &Type{
			Kind: KindMap,
			Key: &Type{
				Kind: KindUnification,
				Uni:  NewElem(),
			},
			Val: &Type{
				Kind: KindUnification,
				Uni:  NewElem(),
			},
		},
		err: true,
	})
	{
		uni0 := NewElem()
		testCases = append(testCases, test{
			name: "same type unification variables in map",
			// the type unification variables should be the same
			typ1: NewType("map{?123:?123}"),
			typ2: &Type{
				Kind: KindMap,
				Key: &Type{
					Kind: KindUnification,
					Uni:  uni0,
				},
				Val: &Type{
					Kind: KindUnification,
					Uni:  uni0,
				},
			},
			err: false,
		})
	}
	{
		uni1 := NewElem()
		uni2 := NewElem()
		uni3 := NewElem()
		// XXX: should we instead have uni0 for the return type and
		// .Union() it with uni2 ?
		//uni0 := NewElem()
		//uni2.Union(uni0)
		testCases = append(testCases, test{
			name: "duplicate type unification variables in functions",
			// the type unification variables should be the same
			typ1: NewType("func(?13, ?42, ?4, int) ?42"),
			typ2: &Type{
				Kind: KindFunc,
				Map: map[string]*Type{
					"a": {
						Kind: KindUnification,
						Uni:  uni1,
					},
					"b": {
						Kind: KindUnification,
						Uni:  uni2,
					},
					"c": {
						Kind: KindUnification,
						Uni:  uni3,
					},
					"d": TypeInt,
				},
				Ord: []string{"a", "b", "c", "d"},
				Out: &Type{
					Kind: KindUnification,
					Uni:  uni2, // same as the second arg
				},
			},
			err: false,
		})
	}
	{
		uni1 := NewElem()
		uni2 := NewElem()
		// XXX: should we instead have uni0 for the return type and
		// .Union() it with uni2 ?
		//uni0 := NewElem()
		//uni2.Union(uni0)
		testCases = append(testCases, test{
			name: "duplicate type unification variables in functions unbalanced",
			// the type unification variables should be the same
			typ1: NewType("func(?13, ?42, ?4, int) ?42"),
			typ2: &Type{
				Kind: KindFunc,
				Map: map[string]*Type{
					"a": {
						Kind: KindUnification,
						Uni:  uni1,
					},
					"b": {
						Kind: KindUnification,
						Uni:  uni2,
					},
					"c": {
						Kind: KindUnification,
						Uni:  uni1, // must not match!
					},
					"d": TypeInt,
				},
				Ord: []string{"a", "b", "c", "d"},
				Out: &Type{
					Kind: KindUnification,
					Uni:  uni2, // same as the second arg
				},
			},
			err: true,
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			typ1, typ2, err := tc.typ1, tc.typ2, tc.err

			// the reverse should probably match the forward version
			err1 := typ1.Cmp(typ2)
			err2 := typ2.Cmp(typ1)

			if err && err1 == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected error, got nil", index)
			}
			if !err && err1 != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unexpected error: %+v", index, err1)
			}
			if err && err2 == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected error, got nil", index)
			}
			if !err && err2 != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unexpected error: %+v", index, err2)
			}
		})
	}
}

func TestTypeOf0(t *testing.T) {
	// TODO: implement testing of the TypeOf function
	// TODO: implement testing TypeOf for struct field name mappings
}

func TestReflect0(t *testing.T) {
	mustPanic := func() (reterr error) {
		defer func() {
			// catch unhandled panics
			if r := recover(); r != nil {
				reterr = fmt.Errorf("panic: %+v", r)
			}
		}()

		// It's unclear if we want this behaviour forever, but it is the
		// current behaviour, and I'd at least like to know if it
		// changes so we can understand where (if at all) it's required.
		typ := NewType("struct{field1 str}")
		_ = typ.Reflect()
		return nil
	}

	if err := mustPanic(); err == nil {
		t.Errorf("expected panic, got nil")
	}
}

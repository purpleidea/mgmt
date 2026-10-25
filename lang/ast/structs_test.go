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

package ast

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/operators"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/wrapped"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

func TestExprCallSpeculationIsolation(t *testing.T) {
	fn := &ExprFunc{
		Function: func() interfaces.Func { return &operators.OperatorFunc{} },
	}
	data := &interfaces.Data{}
	if err := fn.Init(data); err != nil {
		t.Fatal(err)
	}
	call := &ExprCall{
		data: data,
		expr: fn,
		Args: []interfaces.Expr{&ExprStr{V: "+"}, &ExprInt{V: 2}, &ExprInt{V: 3}},
	}
	for i := 0; i < 2; i++ {
		got, err := call.Value()
		if err != nil {
			t.Fatal(err)
		}
		if err := got.Cmp(&types.IntValue{V: 5}); err != nil {
			t.Fatal(err)
		}
		if fn.function.Info().Sig != nil || fn.typ != nil || call.typ != nil {
			t.Fatal("speculation built or typed the original AST")
		}
	}
}

func TestExprCallValuesType(t *testing.T) {
	for _, tc := range []struct {
		name string
		typ  *types.Type
		want *types.Type // nil if the type isn't known yet
	}{
		{"variant arg", types.NewType("func(a variant) str"), types.TypeStr},
		{"variant out", types.NewType("func(a str) variant"), nil},
		{"no type", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			call := &ExprCall{
				expr: &ExprFunc{Values: []*types.FuncValue{{T: tc.typ}}},
			}
			typ, err := call.Type()
			if tc.want == nil {
				if err == nil {
					t.Fatalf("expected an unknown type, got %v", typ)
				}
				return
			}
			if err != nil {
				t.Fatalf("type: %v", err)
			}
			if err := typ.Cmp(tc.want); err != nil {
				t.Fatalf("got %v, want %v: %v", typ, tc.want, err)
			}
		})
	}
}

func TestSpeculateChecksBeforeCall(t *testing.T) {
	for _, tc := range []struct {
		name      string
		sig       string
		args      []types.Value
		impure    bool
		noSpec    bool
		buildErr  bool
		badResult bool
		wantCall  bool
		wantErr   bool
	}{
		{name: "valid", sig: "func(int) int", args: []types.Value{&types.IntValue{V: 1}}, wantCall: true},
		{name: "polymorphic", sig: "func(?1) ?1", args: []types.Value{&types.IntValue{V: 1}}, wantCall: true},
		{name: "wrong kind", sig: "func(int) int", args: []types.Value{&types.StrValue{V: "bad"}}, wantErr: true},
		{name: "missing arg", sig: "func(int) int", args: []types.Value{}, wantErr: true},
		{name: "extra arg", sig: "func() int", args: []types.Value{&types.IntValue{V: 1}}, wantErr: true},
		{name: "repeated variable", sig: "func(?1, ?1) int", args: []types.Value{&types.IntValue{V: 1}, &types.StrValue{V: "bad"}}, wantErr: true},
		{name: "unknown result", sig: "func(int) ?1", args: []types.Value{&types.IntValue{V: 1}}, wantErr: true},
		{name: "build rejects", sig: "func(?1) int", args: []types.Value{&types.IntValue{V: 1}}, buildErr: true, wantErr: true},
		{name: "impure", sig: "func(int) int", args: []types.Value{&types.IntValue{V: 1}}, impure: true, wantErr: true},
		// A pure function can still opt out of running at compile time.
		{name: "not spec", sig: "func(int) int", args: []types.Value{&types.IntValue{V: 1}}, noSpec: true, wantErr: true},
		{name: "bad result", sig: "func(int) int", args: []types.Value{&types.IntValue{V: 1}}, badResult: true, wantCall: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called, built := false, false
			fn := &simple.Func{
				WrappedFunc: &wrapped.Func{
					Name:     "speculation test",
					FuncInfo: &wrapped.Info{Pure: !tc.impure, Spec: !tc.noSpec},
					Type:     types.NewType(tc.sig),
				},
				Check: func(typ *types.Type) error {
					if typ.HasUni() {
						t.Fatal("build received an unresolved signature")
					}
					if tc.buildErr {
						return fmt.Errorf("unsupported signature")
					}
					built = true
					return nil
				},
				Func: func(ctx context.Context, args []types.Value) (types.Value, error) {
					called = true
					if !built {
						t.Fatal("call ran before build")
					}
					if tc.badResult {
						return &types.StrValue{V: "bad"}, nil
					}
					return &types.IntValue{V: args[0].Int()}, nil
				},
			}
			_, err := funcs.Speculate(context.Background(), fn, tc.args, nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if called != tc.wantCall {
				t.Fatalf("call ran: %t, want: %t", called, tc.wantCall)
			}
		})
	}
}

// samePosition reports whether these two textareas cover the same position.
func samePosition(a, b interfaces.Textarea) bool {
	al, ac := a.Pos()
	bl, bc := b.Pos()
	ael, aec := a.End()
	bel, bec := b.End()
	return al == bl && ac == bc && ael == bel && aec == bec
}

func TestExprMapKeepsEntryPositions(t *testing.T) {
	kv := &ExprMapKV{
		Key: &ExprVar{Name: "k"},
		Val: &ExprVar{Name: "v"},
	}
	kv.Locate(1, 2, 3, 4)
	obj := &ExprMap{KVs: []*ExprMapKV{kv}}

	interpolated, err := obj.Interpolate()
	if err != nil {
		t.Fatalf("func Interpolate failed: %+v", err)
	}
	if x := interpolated.(*ExprMap).KVs[0]; !samePosition(x.Textarea, kv.Textarea) {
		t.Errorf("func Interpolate lost the position of the map entry")
	}

	copied, err := obj.Copy()
	if err != nil {
		t.Fatalf("func Copy failed: %+v", err)
	}
	if copied == obj {
		t.Fatalf("func Copy did not copy the map")
	}
	if x := copied.(*ExprMap).KVs[0]; !samePosition(x.Textarea, kv.Textarea) {
		t.Errorf("func Copy lost the position of the map entry")
	}
}

func TestExprMapInitUnsolvedKeyType(t *testing.T) {
	// A param whose type isn't solved yet, so it might still be hashable.
	param := func() *ExprParam {
		return &ExprParam{
			Name: "x",
			typ:  &types.Type{Kind: types.KindUnification, Uni: types.NewElem()},
		}
	}
	for _, key := range []interfaces.Expr{
		param(),
		&ExprStruct{Fields: []*ExprStructField{{Name: "a", Value: param()}}},
	} {
		obj := &ExprMap{KVs: []*ExprMapKV{{Key: key, Val: &ExprInt{V: 1}}}}
		if err := obj.Init(&interfaces.Data{}); err != nil {
			t.Errorf("func Init failed: %+v", err)
		}
	}
}

func TestExprStructKeepsFieldPositions(t *testing.T) {
	field := &ExprStructField{
		Name:  "f",
		Value: &ExprVar{Name: "v"},
	}
	field.Locate(1, 2, 3, 4)
	obj := &ExprStruct{Fields: []*ExprStructField{field}}

	interpolated, err := obj.Interpolate()
	if err != nil {
		t.Fatalf("func Interpolate failed: %+v", err)
	}
	if x := interpolated.(*ExprStruct).Fields[0]; !samePosition(x.Textarea, field.Textarea) {
		t.Errorf("func Interpolate lost the position of the struct field")
	}

	copied, err := obj.Copy()
	if err != nil {
		t.Fatalf("func Copy failed: %+v", err)
	}
	if copied == obj {
		t.Fatalf("func Copy did not copy the struct")
	}
	if x := copied.(*ExprStruct).Fields[0]; !samePosition(x.Textarea, field.Textarea) {
		t.Errorf("func Copy lost the position of the struct field")
	}
}

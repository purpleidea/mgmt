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

package resources

import (
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	utilJSON "github.com/purpleidea/mgmt/util/json"
)

// TestJSONRoundTrip encodes every registered resource kind, with non-default
// values in all of its trait meta params, and checks that decoding it into a
// fresh resource of the same kind gives back an identical resource. This
// catches exported trait fields which collide when an encoder flattens the
// embedded traits into the resource, since those get silently dropped. It also
// checks that every field type in use, such as the complex numbers of the test
// resource, can be encoded with our json options.
func TestJSONRoundTrip(t *testing.T) {
	for _, kind := range engine.RegisteredResourcesNames() {
		t.Run(kind, func(t *testing.T) {
			res, err := engine.NewNamedResource(kind, "probe")
			if err != nil {
				t.Fatalf("func NewNamedResource: %v", err)
			}
			res = res.Default()
			res.SetKind(kind)
			res.SetName("probe")

			meta := engine.DefaultMetaParams.Copy()
			meta.Retry = 3
			meta.Sema = []string{"a:1"}
			meta.Export = []string{"*"}
			res.SetMetaParams(meta)
			if r, ok := res.(engine.EdgeableRes); ok {
				r.SetAutoEdgeMeta(&engine.AutoEdgeMeta{Disabled: true})
			}
			if r, ok := res.(engine.GroupableRes); ok {
				r.SetAutoGroupMeta(&engine.AutoGroupMeta{Disabled: true})
			}
			if r, ok := res.(engine.ReversibleRes); ok {
				r.SetReversibleMeta(&engine.ReversibleMeta{
					Disabled:  true,
					Reversal:  true,
					Overwrite: true,
				})
			}
			if r, ok := res.(*TestRes); ok {
				r.Complex64 = 1.5 + 2i
				r.Complex128 = -3 + 0.25i
			}

			b, err := utilJSON.Marshal(res)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			out, err := engine.NewResource(kind)
			if err != nil {
				t.Fatalf("func NewResource: %v", err)
			}
			if err := utilJSON.Unmarshal(b, out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !reflect.DeepEqual(res, out) {
				t.Errorf("round trip differs, encoded as: %s", b)
			}
		})
	}
}

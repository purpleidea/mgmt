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

package corenet

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func TestMacFmt(t *testing.T) {
	var tests = []struct {
		name    string
		in      string
		out     string
		wantErr bool
	}{
		{"Valid mac with hyphens", "01-23-45-67-89-AB", "01:23:45:67:89:ab", false},
		{"Valid mac with colons", "01:23:45:67:89:AB", "01:23:45:67:89:ab", false},
		{"Incorrect mac length with colons", "01:23:45:67:89:AB:01:23:45:67:89:AB", "01:23:45:67:89:AB:01:23:45:67:89:AB", true},
		{"Invalid mac", "", "", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			m, err := MacFmt(context.Background(), []types.Value{&types.StrValue{V: tt.in}})
			if (err != nil) != tt.wantErr {
				t.Errorf("func MacFmt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if m != nil {
				if err := m.Cmp(&types.StrValue{V: tt.out}); err != nil {
					t.Errorf("got %q, want %q", m.Value(), tt.out)
				}
			}
		})
	}
}

func TestOldMacFmt(t *testing.T) {
	var tests = []struct {
		name    string
		in      string
		out     string
		wantErr bool
	}{
		{"Valid mac with hyphens", "01:23:45:67:89:AB", "01-23-45-67-89-ab", false},
		{"Valid lowercase mac with hyphens", "01:23:45:67:89:ab", "01-23-45-67-89-ab", false},
		{"Valid mac with colons", "01-23-45-67-89-AB", "01-23-45-67-89-ab", false},
		{"Incorrect mac length with hyphens", "01-23-45-67-89-AB-01-23-45-67-89-AB", "01-23-45-67-89-AB-01-23-45-67-89-AB", true},
		{"Invalid mac", "", "", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			m, err := OldMacFmt(context.Background(), []types.Value{&types.StrValue{V: tt.in}})
			if (err != nil) != tt.wantErr {
				t.Errorf("func MacFmt() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if m != nil {
				if err := m.Cmp(&types.StrValue{V: tt.out}); err != nil {
					t.Errorf("got %q, want %q", m.Value(), tt.out)
				}
			}
		})
	}
}

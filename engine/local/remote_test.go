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

package local

import (
	"testing"
)

func TestRemoteEncode(t *testing.T) {
	remote := &Remote{
		Parent: "h1",
	}
	s, err := remote.Encode()
	if err != nil {
		t.Fatalf("func Encode: %v", err)
	}
	if exp := `{"parent":"h1"}`; s != exp {
		t.Errorf("func Encode: got: %s, expected: %s", s, exp)
	}

	decoded, err := DecodeRemote(s)
	if err != nil {
		t.Fatalf("func DecodeRemote: %v", err)
	}
	if *decoded != *remote {
		t.Errorf("func DecodeRemote: got: %+v, expected: %+v", decoded, remote)
	}

	if _, err := (&Remote{}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an empty parent")
	}
	if _, err := (&Remote{Parent: "bad host"}).Encode(); err == nil {
		t.Errorf("func Encode: expected error for an invalid parent")
	}
}

func TestRemoteDecodeInvalid(t *testing.T) {
	invalid := []string{
		``,
		`{}`,
		`null`,
		`{"parent":null}`,
		`{"parent":""}`,
		`{"parent":"bad host"}`,
		`{"parent":"h1","evil":1}`,       // unknown member
		`{"PARENT":"h1"}`,                // case mismatch
		`{"parent":"h1","parent":"h2"}`,  // duplicate member
		"{\"parent\":\"h\xff1\"}",        // invalid UTF-8
		`{"parent":"h1"} {}`,             // trailing data
		`{"parent":"h1"}{"parent":"h2"}`, // trailing data
		` {"parent":"h1"}`,               // whitespace
		`{"parent":"h1"} `,               // whitespace
		`{"parent": "h1"}`,               // whitespace
		"{\"\\u0070arent\":\"h1\"}",      // escaped member name
		"{\"parent\":\"h\\u0031\"}",      // escaped value
		`{"parent":1}`,
		`{"parent":["h1"]}`,
		`["h1"]`,
		`"h1"`,
		`h1`,
	}
	for _, s := range invalid {
		if remote, err := DecodeRemote(s); err == nil {
			t.Errorf("func DecodeRemote(%q): expected error, got: %+v", s, remote)
		}
	}
}

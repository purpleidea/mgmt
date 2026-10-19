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

package remote

import (
	"testing"
)

func TestDonePath(t *testing.T) {
	path, err := DonePath("child1", "parent1", "ssh://root@h1:22/")
	if err != nil {
		t.Fatalf("func DonePath: %v", err)
	}
	if exp := "/remote/done/child1/parent1/ssh:%2F%2Froot@h1:22%2F"; path != exp {
		t.Errorf("func DonePath: got: %s, expected: %s", path, exp)
	}

	invalid := [][]string{
		{"", "parent1", "r0"},
		{"child1", "", "r0"},
		{"child1", "parent1", ""},
		{"child/1", "parent1", "r0"},
		{"child1", "parent/1", "r0"},
		{"..", "parent1", "r0"},
		{"child1", "*", "r0"},
		{"child 1", "parent1", "r0"},
	}
	for _, x := range invalid {
		if path, err := DonePath(x[0], x[1], x[2]); err == nil {
			t.Errorf("func DonePath(%q): expected error, got: %s", x, path)
		}
	}
}

func TestAckPath(t *testing.T) {
	path, err := AckPath("parent1", "a/b")
	if err != nil {
		t.Fatalf("func AckPath: %v", err)
	}
	if exp := "/remote/ack/parent1/a%2Fb"; path != exp {
		t.Errorf("func AckPath: got: %s, expected: %s", path, exp)
	}

	// The escaped name is one segment, so one name can never be a prefix
	// match, or an escaped form, of another.
	p1, err := AckPath("parent1", "a/b")
	if err != nil {
		t.Fatalf("func AckPath: %v", err)
	}
	p2, err := AckPath("parent1", "a%2Fb")
	if err != nil {
		t.Fatalf("func AckPath: %v", err)
	}
	if p1 == p2 {
		t.Errorf("func AckPath: different names gave the same path: %s", p1)
	}

	if path, err := AckPath("", "r0"); err == nil {
		t.Errorf("func AckPath: expected error for an empty parent, got: %s", path)
	}
	if path, err := AckPath("parent1", ""); err == nil {
		t.Errorf("func AckPath: expected error for an empty name, got: %s", path)
	}
}

// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package util_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	engineutil "github.com/purpleidea/mgmt/engine/util"
)

func TestSymbolicMode(t *testing.T) {
	def := os.FileMode(0644) | os.ModeSetgid
	symModeTests := []struct {
		name       string
		input      []string
		expect     os.FileMode
		onlyAssign bool
		err        error
	}{
		// Test single mode inputs.
		{"assign", []string{"a=rwx"}, 0777, false, nil},
		{"assign", []string{"ug=rwx"}, 0774, false, nil},
		{"assign", []string{"ug=srwx"}, 0774 | os.ModeSetgid | os.ModeSetuid, false, nil},
		{"assign", []string{"ug=trwx"}, 0774 | os.ModeSticky, false, nil},
		{"assign", []string{"o=rx"}, 0645 | os.ModeSetgid, false, nil},
		{"assign", []string{"ug=srwx"}, 0774 | os.ModeSetgid | os.ModeSetuid, false, nil},
		{"addition", []string{"o+rwx"}, 0647 | os.ModeSetgid, false, nil},
		{"addition", []string{"u+x"}, 0744 | os.ModeSetgid, false, nil},
		{"addition", []string{"u+x"}, 0744 | os.ModeSetgid, false, nil},
		{"addition", []string{"u+s"}, 0644 | os.ModeSetgid | os.ModeSetuid, false, nil},
		{"addition", []string{"u+t"}, 0644 | os.ModeSetgid | os.ModeSticky, false, nil},
		{"subtraction", []string{"o-rwx"}, 0640 | os.ModeSetgid, false, nil},
		{"subtraction", []string{"u-w"}, 0444 | os.ModeSetgid, false, nil},
		{"subtraction", []string{"g-s"}, 0644, false, nil},
		{"subtraction", []string{"u-t"}, 0644 | os.ModeSetgid, false, nil},

		// Test multiple mode inputs.
		{"mixed", []string{"u=rwx", "g+w"}, 0764 | os.ModeSetgid, false, nil},
		{"mixed", []string{"u+rwx", "g=w"}, 0724, false, nil},

		// Test that a engineutil.ModeError is returned. Value is not checked so the
		// empty string works.
		{"invalid separator", []string{"ug_rwx"}, os.FileMode(0), false, fmt.Errorf("ug_rwx is not a valid a symbolic mode")},
		{"invalid who", []string{"xg=rwx"}, os.FileMode(0), false, fmt.Errorf("unexpected character assignment in xg=rwx")},
		{"invalid what", []string{"g=rwy"}, os.FileMode(0), false, fmt.Errorf("unexpected character assignment in g=rwy")},
		{"double assignment", []string{"a=rwx", "u=r"}, os.FileMode(0), false, fmt.Errorf("subject was repeated: each subject (u,g,o) is only accepted once")},

		// Test onlyAssign bool
		{"only assign", []string{"u+x", "g=rw"}, os.FileMode(0), true, fmt.Errorf("u+x is not a valid a symbolic mode")},
		{"not only assign", []string{"u+x", "g=rw"}, os.FileMode(0764), false, nil},
	}

	for _, ts := range symModeTests {
		test := ts
		t.Run(test.name+" "+strings.Join(test.input, ","), func(t *testing.T) {
			got, err := engineutil.ParseSymbolicModes(test.input, def, test.onlyAssign)
			if test.err != nil {
				if err == nil {
					t.Errorf("input: %s, expected error: %#v, but got nil", def, test.err)
				} else if err.Error() != test.err.Error() {
					t.Errorf("input: %s, expected error: %q, got: %q", def, test.err, err)
				}
			} else if test.err == nil && err != nil {
				t.Errorf("input: %s, did not expect error but got: %#v", def, err)
			}

			// Verify we get the expected value (including zero on error).
			if test.expect != got {
				t.Errorf("input: %s, expected: %v, got: %v", def, test.expect, got)
			}
		})
	}
}

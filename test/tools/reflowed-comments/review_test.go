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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReflowReview documents proposed behavior for review. These cases expose
// gaps in the current implementation; their expectations await confirmation.
func TestReflowReview(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	testCases := []struct {
		name         string
		source       string
		want         string
		rejectBefore bool
	}{
		{
			name: "interface_method",
			source: `package doc

type T interface {
	// Disabled specifies that automatic grouping should be disabled for this
	// resource.
	Disabled() bool
}
`,
			want: `package doc

type T interface {
	// Disabled specifies that automatic grouping should be disabled for
	// this resource.
	Disabled() bool
}
`,
			rejectBefore: true,
		},
		{
			name: "nested_struct_field",
			source: `package doc

type T struct {
	Options struct {
		// Disabled specifies that automatic grouping should be disabled for this
		// resource.
		Disabled bool
	}
}
`,
			want: `package doc

type T struct {
	Options struct {
		// Disabled specifies that automatic grouping should be disabled
		// for this resource.
		Disabled bool
	}
}
`,
			rejectBefore: true,
		},
		{
			name: "anonymous_struct_variable",
			source: `package doc

var Options struct {
	// Disabled specifies that automatic grouping should be disabled for this
	// resource.
	Disabled bool
}
`,
			want: `package doc

var Options struct {
	// Disabled specifies that automatic grouping should be disabled for
	// this resource.
	Disabled bool
}
`,
			rejectBefore: true,
		},
		{
			name:   "space_then_tab_indented_code",
			source: "package doc\n\n// F demonstrates a command sequence.\n//\n// \techo  one\n// \techo  two\nfunc F() {}\n",
		},
		{
			name: "asterisk_bullets",
			source: `package doc

// F supports these modes:
//
// * first mode
// * second mode
func F() {}
`,
		},
		{
			name: "digit_in_prose_word",
			source: `package doc

// F configures
// IPv6 routing.
func F() {}
`,
			want: `package doc

// F configures IPv6 routing.
func F() {}
`,
			rejectBefore: true,
		},
		{
			name:   "spaces_inside_inline_code",
			source: "package doc\n\n// F runs `printf \"a  b\"` after setup.\nfunc F() {}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.want
			if want == "" {
				want = tc.source // formatting should be preserved
			}
			if err := os.WriteFile(filename, []byte(tc.source), 0600); err != nil {
				t.Fatalf("could not write test file: %+v", err)
			}
			if tc.rejectBefore && Check(filename) == nil {
				t.Errorf("expected original comment to fail Check")
			}
			if err := Reflow(filename); err != nil {
				t.Errorf("expected Reflow to succeed: %+v", err)
			}
			result, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("could not read test file: %+v", err)
			}
			if string(result) != want {
				t.Errorf("unexpected reflow result:\n%s\nwant:\n%s", result, want)
			}
			if err := Check(filename); err != nil {
				t.Errorf("expected reflowed comment to pass Check: %+v", err)
			}
		})
	}
}

// TestCheckIndentReview requires code blocks to use tabs instead of spaces.
func TestCheckIndentReview(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "doc.go")
	data := `package doc

// F demonstrates a command sequence.
//
//     echo  one
//     echo  two
func F() {}
`
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}
	err := Check(filename)
	if err == nil {
		t.Fatalf("expected space-indented code to fail Check")
	}
	if !strings.Contains(err.Error(), "wasn't trimmed properly") {
		t.Fatalf("expected an indentation error: %+v", err)
	}
}

// TestNumberBulletReview distinguishes list markers from prose with digits.
func TestNumberBulletReview(t *testing.T) {
	for _, word := range []string{"IPv6", "sha256", "v1.2"} {
		t.Run(word, func(t *testing.T) {
			if IsNumberBullet(word) {
				t.Errorf("%q is prose, not a numbered bullet", word)
			}
		})
	}
	for _, word := range []string{"1.", "42)"} {
		t.Run(word, func(t *testing.T) {
			if !IsNumberBullet(word) {
				t.Errorf("%q should be recognized as a numbered bullet", word)
			}
		})
	}
}

// TestCheckLongFilenameReview requires a diagnostic instead of a panic.
func TestCheckLongFilenameReview(t *testing.T) {
	filename := filepath.Join(t.TempDir(), strings.Repeat("x", 80)+".go")
	data := "package doc\n\n// F has " + strings.Repeat("many words ", 12) + "in its comment.\nfunc F() {}\n"
	if err := os.WriteFile(filename, []byte(data), 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("the Check should report a comment error without panicking: %v", r)
		}
	}()
	if err := Check(filename); err == nil {
		t.Errorf("expected overflowing comment to fail Check")
	}
}

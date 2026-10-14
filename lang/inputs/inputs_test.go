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

package inputs

import (
	"bytes"
	"testing"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"

	"github.com/spf13/afero"
)

func newTestFs() *util.AferoFs {
	return &util.AferoFs{Afero: &afero.Afero{Fs: afero.NewMemMapFs()}}
}

// TestParseInputNewline checks that every kind of input ends up with a trailing
// newline in Main, because the parser needs one after the last statement, and
// that input which already has one is left alone.
func TestParseInputNewline(t *testing.T) {
	code := `print "hello" { msg => "world", }`
	comment := code + " # no newline after this comment"

	tests := []struct {
		name  string
		code  string
		exp   string
		setup func(fs *util.AferoFs) (string, error) // returns the input arg
	}{
		{
			name: "inline code without newline",
			code: code,
			exp:  code + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				return code, nil
			},
		},
		{
			name: "inline code with newline",
			code: code + "\n",
			exp:  code + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				return code + "\n", nil
			},
		},
		{
			name: "inline code with two newlines",
			code: code + "\n\n",
			exp:  code + "\n\n",
			setup: func(fs *util.AferoFs) (string, error) {
				return code + "\n\n", nil
			},
		},
		{
			name: "inline code ending in a comment",
			code: comment,
			exp:  comment + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				return comment, nil
			},
		},
		{
			name: "mcl file without newline",
			code: code,
			exp:  code + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				p := "/" + interfaces.MainFilename
				return p, fs.WriteFile(p, []byte(code), 0600)
			},
		},
		{
			name: "mcl file with newline",
			code: code + "\n",
			exp:  code + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				p := "/" + interfaces.MainFilename
				return p, fs.WriteFile(p, []byte(code+"\n"), 0600)
			},
		},
		{
			name: "metadata file without newline",
			code: code,
			exp:  code + "\n",
			setup: func(fs *util.AferoFs) (string, error) {
				m := "/" + interfaces.MainFilename
				if err := fs.WriteFile(m, []byte(code), 0600); err != nil {
					return "", err
				}
				p := "/" + interfaces.MetadataFilename
				y := "main: " + interfaces.MainFilename + "\n"
				return p, fs.WriteFile(p, []byte(y), 0600)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := newTestFs()
			input, err := tt.setup(fs)
			if err != nil {
				t.Fatalf("setup failed: %+v", err)
			}

			output, err := ParseInput(input, fs)
			if err != nil {
				t.Fatalf("parse input failed: %+v", err)
			}
			if got := string(output.Main); got != tt.exp {
				t.Fatalf("unexpected Main:\ngot: %q\nexp: %q", got, tt.exp)
			}

			// Whatever the workers write out must match what we parse,
			// otherwise a deploy would carry a different program.
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Fatalf("worker failed: %+v", err)
				}
			}
			if len(output.Files) != 0 { // read from disk, not copied
				return
			}
			b, err := fs.ReadFile("/" + output.Metadata.Main)
			if err != nil {
				t.Fatalf("could not read back main file: %+v", err)
			}
			if !bytes.Equal(b, output.Main) {
				t.Fatalf("copied main file differs:\ngot: %q\nexp: %q", b, output.Main)
			}
		})
	}
}

func TestNewlineTerminated(t *testing.T) {
	tests := []struct {
		in  string
		exp string
	}{
		{"", ""},
		{"\n", "\n"},
		{"x", "x\n"},
		{"x\n", "x\n"},
		{"x\n\n", "x\n\n"},
		{"x\r\n", "x\r\n"},
	}
	for _, tt := range tests {
		if got := string(newlineTerminated([]byte(tt.in))); got != tt.exp {
			t.Errorf("newlineTerminated(%q) = %q, exp %q", tt.in, got, tt.exp)
		}
	}
}

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

package interfaces

import (
	"testing"
)

// testFS is a minimal, read-only filesystem which fulfills the engine.ReadFS
// interface. We only need it to return a URI here.
type testFS struct {
	uri string
}

// URI returns the unique handle for this filesystem.
func (obj *testFS) URI() string { return obj.uri }

// ReadFile returns the contents of the named file in this filesystem.
func (obj *testFS) ReadFile(name string) ([]byte, error) {
	return []byte(obj.uri + name), nil
}

func TestSourceFileFilename(t *testing.T) {
	tests := []struct {
		name string
		file *SourceFile
		uri  string
		out  string
	}{
		{
			name: "no filesystem",
			file: &SourceFile{Path: "/main.mcl"},
			uri:  "/main.mcl",
			out:  "/main.mcl",
		},
		{
			name: "local disk",
			file: &SourceFile{
				FS:   &testFS{uri: "ReadOnlyFilter:///"},
				Path: "/home/james/code/main.mcl",
			},
			uri: "ReadOnlyFilter:///home/james/code/main.mcl",
			out: "/home/james/code/main.mcl", // unchanged!
		},
		{
			name: "deploy",
			file: &SourceFile{
				FS:   &testFS{uri: "etcdfs:///_mgmt/deploy/1"},
				Path: "/main.mcl",
			},
			uri: "etcdfs:///_mgmt/deploy/1/main.mcl",
			out: "/main.mcl", // unchanged!
		},
		{
			// Without the module in here, this would be identical
			// to the /main.mcl of every other embedded module.
			name: "embedded",
			file: &SourceFile{
				FS:   &testFS{uri: EmbeddedScheme + ":///embedded/provisioner"},
				Path: "/main.mcl",
			},
			uri: "embeddedfs:///embedded/provisioner/main.mcl",
			out: "embeddedfs:///embedded/provisioner/main.mcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s := tt.file.URI(); s != tt.uri {
				t.Errorf("func URI: got %s, expected %s", s, tt.uri)
			}
			if s := tt.file.Filename(); s != tt.out {
				t.Errorf("func Filename: got %s, expected %s", s, tt.out)
			}
			// The String method is always the plain path.
			if s := tt.file.String(); s != tt.file.Path {
				t.Errorf("func String: got %s, expected %s", s, tt.file.Path)
			}
		})
	}
}

func TestSourceFileNil(t *testing.T) {
	var file *SourceFile // nil

	if s := file.URI(); s != "" {
		t.Errorf("func URI: got %s, expected empty", s)
	}
	if s := file.Filename(); s != "" {
		t.Errorf("func Filename: got %s, expected empty", s)
	}
	if s := file.String(); s != "" {
		t.Errorf("func String: got %s, expected empty", s)
	}
	if _, err := file.Source(); err == nil {
		t.Errorf("func Source: expected an error")
	}

	// A Data struct without a filesystem must not panic either.
	var data *Data // nil
	if f := data.SourceFile(); f == nil {
		t.Errorf("func SourceFile: got nil")
	}
}

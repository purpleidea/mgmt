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
	"testing"
)

func TestCheckPackageDoc(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`// Package doc has a comment which is split poorly enough for this checker.
// It should reject the second line because its first word fits above.
package doc
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err == nil {
		t.Fatalf("expected package doc reflow error")
	}
}

func TestCheckPackageDocAllowsTabCodeBlock(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`// Package doc has a code block.
//
//	./mgmt run --tmp-prefix --no-pgp --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381 --server-urls=http://127.0.0.1:2382 empty
package doc
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected package doc tab code block to pass: %+v", err)
	}
}

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

func TestReflow(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`// Package doc has a comment which is split poorly enough for this checker.
// It should reject the second line because its first word fits above.  This sentence is also much too long to fit on one line without being wrapped.
//
//
// NOTE:   Keep this as a new paragraph.
//
//	./mgmt run --tmp-prefix --no-pgp --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381
package doc
`)
	want := `// Package doc has a comment which is split poorly enough for this checker. It
// should reject the second line because its first word fits above. This
// sentence is also much too long to fit on one line without being wrapped.
//
// NOTE: Keep this as a new paragraph.
//
//	./mgmt run --tmp-prefix --no-pgp --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381
package doc
`

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err == nil {
		t.Fatalf("expected package doc reflow error")
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != want {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected reflowed package doc to pass: %+v", err)
	}
}

func TestReflowPreservesDirective(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

// F is a function with a comment that needs to be reflowed onto the next line because it is too long.
//go:noinline
func F() {}
`)
	want := `package doc

// F is a function with a comment that needs to be reflowed onto the next line
// because it is too long.
//go:noinline
func F() {}
`

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != want {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestReflowAllowsLongURL(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

// F was modified from: https://github.com/coredhcp/coredhcp/blob/b4aa45e6f7268cc4c52f863b130bd8eb388647b2/plugins/leasetime/plugin.go#L32
func F() {}
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected long URL to pass: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != string(data) {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestReflowDoesNotMoveLongURL(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

// F was modified from:
// http://docs.aws.amazon.com/cli/latest/userguide/cli-config-files.html
func F() {}

// T is our internal copy of a struct as found here:
// https://godocs.io/github.com/coreos/etcd/etcdserver/etcdserverpb#Member but
// which uses native types where possible.
type T struct{}
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected standalone URLs to pass: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != string(data) {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

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

func TestCheckStructFieldDoc(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

type T struct {
	// InstallDeps specifies whether we should run a generated script on
	// the remote host which installs the runtime dependencies of this
	// binary (such as the augeas and libvirt libraries) before we start
	// the remote process. This usually requires root permissions there.
	InstallDeps bool
}
`)
	want := `package doc

type T struct {
	// InstallDeps specifies whether we should run a generated script on the
	// remote host which installs the runtime dependencies of this binary
	// (such as the augeas and libvirt libraries) before we start the remote
	// process. This usually requires root permissions there.
	InstallDeps bool
}
`

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	err := Check(filename)
	if err == nil {
		t.Fatalf("expected struct field doc reflow error")
	}
	if !strings.Contains(err.Error(), filename+":4") {
		t.Fatalf("expected full path in reflow error: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != want {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestStructFieldDocAccountsForIndent(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

type T struct {
	// Disabled specifies that automatic grouping should be disabled for this
	// resource.
	Disabled bool
}
`)
	want := `package doc

type T struct {
	// Disabled specifies that automatic grouping should be disabled for
	// this resource.
	Disabled bool
}
`

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err == nil {
		t.Fatalf("expected indented struct field doc reflow error")
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != want {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestConstSpecDoc(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

const (
	// InstallDeps specifies whether we should run a generated script on
	// the remote host which installs the runtime dependencies of this
	// binary (such as the augeas and libvirt libraries) before we start
	// the remote process. This usually requires root permissions there.
	InstallDeps = true
)
`)
	want := `package doc

const (
	// InstallDeps specifies whether we should run a generated script on the
	// remote host which installs the runtime dependencies of this binary
	// (such as the augeas and libvirt libraries) before we start the remote
	// process. This usually requires root permissions there.
	InstallDeps = true
)
`

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err == nil {
		t.Fatalf("expected const spec doc reflow error")
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != want {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestReflowPreservesCommentedOutConstSpec(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

const (
	// Current is enabled.
	//disabled = false
	Current = true
)
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected commented-out const spec to pass: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != string(data) {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestCheckIgnoresLocalConstSpecDoc(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

func F() {
	const (
		// Local is a constant with a comment that is deliberately split
		// too early.
		Local = true
	)
}
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected local const spec doc to be ignored: %+v", err)
	}
}

func TestReflowPreservesCommentedOutStructField(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

type T struct {
	// Current is enabled.
	//disabled bool
	Current bool
}
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected commented-out struct field to pass: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != string(data) {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestReflowPreservesBUGParagraph(t *testing.T) {
	dir := t.TempDir()
	filename := filepath.Join(dir, "doc.go")
	data := []byte(`package doc

type T struct {
	// Interface is the interface to bind to.
	// XXX: An interface must currently be specified.
	// BUG: https://github.com/example/project/issues/example
	Interface string
}
`)

	if err := os.WriteFile(filename, data, 0600); err != nil {
		t.Fatalf("could not write test file: %+v", err)
	}

	if err := Check(filename); err != nil {
		t.Fatalf("expected BUG paragraph to pass: %+v", err)
	}

	if err := Reflow(filename); err != nil {
		t.Fatalf("could not reflow test file: %+v", err)
	}

	result, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("could not read test file: %+v", err)
	}
	if string(result) != string(data) {
		t.Fatalf("unexpected reflow result:\n%s", result)
	}
}

func TestReflowPreservesInlineCodeSpan(t *testing.T) {
	lines := []string{
		"Run this command after the previous operation succeeds: `$cmd && echo done > /tmp/donefile`. Then continue.",
	}
	want := []string{
		"Run this command after the previous operation succeeds:",
		"`$cmd && echo done > /tmp/donefile`. Then continue.",
	}

	result := reflowLines(lines, maxLength)
	if len(result) != len(want) {
		t.Fatalf("unexpected reflow result: %#v", result)
	}
	for i := range result {
		if result[i] != want[i] {
			t.Fatalf("unexpected reflow result: %#v", result)
		}
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

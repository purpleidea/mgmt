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

package format_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/embedded"
	"github.com/purpleidea/mgmt/lang/format"
	"github.com/purpleidea/mgmt/lang/format/astfmt"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/interpolate"
	"github.com/purpleidea/mgmt/lang/parser"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	godiff "github.com/kylelemons/godebug/diff"
	"github.com/spf13/afero"
	"golang.org/x/tools/txtar"
)

type txtarConfig struct {
	NoFmt bool `json:"nofmt"`
}

// newFormatter returns a formatter with the real parser and printer wired in.
// This test file is in a separate _test package so that it can import those
// directly, which the format package itself can't do, since that would be an
// import cycle.
func newFormatter(t *testing.T) *format.Formatter {
	formatter := &format.Formatter{
		LexParser:    parser.LexParseWithComments,
		ASTFormatter: astfmt.Format,
		Logf: func(format string, v ...interface{}) {
			t.Logf("formatter: "+format, v...)
		},
	}
	formatter.Init()
	return formatter
}

// TestFmtFiles is the main test harness for the formatter. It looks at every
// file in the tests/ directory. An .mcl file must be already formatted in the
// canonical style, and formatting it must return it unchanged. A .badmcl file
// must have an .mcl file of the same name next to it, and formatting the
// .badmcl contents must produce exactly the .mcl contents. To add a new test,
// just drop in a new file pair.
func TestFmtFiles(t *testing.T) {
	dir := "tests/"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("could not read tests dir: %+v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		if strings.HasSuffix(name, ".mcl") {
			t.Run(name, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatalf("could not read file: %+v", err)
				}

				formatter := newFormatter(t)
				output, err := formatter.FormatData(context.TODO(), bytes.NewReader(data))
				if err != nil {
					t.Fatalf("func FormatData failed: %+v", err)
				}
				if !bytes.Equal(output, data) {
					t.Errorf("file is not in canonical format: %s", name)
					t.Logf("diff:\n%s", godiff.Diff(string(data), string(output)))
				}
			})
			continue
		}

		if strings.HasSuffix(name, ".badmcl") {
			t.Run(name, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatalf("could not read file: %+v", err)
				}

				goodName := strings.TrimSuffix(name, ".badmcl") + ".mcl"
				expected, err := os.ReadFile(filepath.Join(dir, goodName))
				if err != nil {
					t.Fatalf("missing the %s pair file: %+v", goodName, err)
				}

				formatter := newFormatter(t)
				output, err := formatter.FormatData(context.TODO(), bytes.NewReader(data))
				if err != nil {
					t.Fatalf("func FormatData failed: %+v", err)
				}
				if !bytes.Equal(output, expected) {
					t.Errorf("unexpected format output for: %s", name)
					t.Logf("diff:\n%s", godiff.Diff(string(expected), string(output)))
				}
			})
			continue
		}

		t.Errorf("unexpected file in tests dir: %s", name)
	}
}

func TestFmtTxtarFiles(t *testing.T) {
	root := filepath.Clean("../..")
	formatter := newFormatter(t)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".txtar") {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		t.Run(rel, func(t *testing.T) {
			archive, err := txtar.ParseFile(path)
			if err != nil {
				t.Fatalf("err parsing txtar(%s): %+v", path, err)
			}

			var config txtarConfig
			for _, file := range archive.Files {
				if file.Name != "CONFIG" {
					continue
				}
				if err := json.Unmarshal(file.Data, &config); err != nil {
					t.Fatalf("err parsing txtar(%s) config: %+v", path, err)
				}
				break
			}
			if config.NoFmt {
				t.Skip("nofmt")
			}

			for _, file := range archive.Files {
				if !strings.HasSuffix(file.Name, ".mcl") {
					continue
				}

				t.Run(file.Name, func(t *testing.T) {
					output, err := formatter.FormatData(context.TODO(), bytes.NewReader(file.Data))
					if err != nil {
						t.Fatalf("func FormatData failed: %+v", err)
					}
					if !bytes.Equal(output, file.Data) {
						t.Errorf("file is not in canonical format: %s", file.Name)
						t.Logf("diff:\n%s", godiff.Diff(string(file.Data), string(output)))
					}
				})
			}
		})

		return nil
	})
	if err != nil {
		t.Fatalf("func WalkDir failed: %+v", err)
	}
}

func TestFormatData(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "already formatted",
			input: "$x = 42\n",
			want:  "$x = 42\n",
		},
		{
			name:  "missing final newline",
			input: "$x = 42",
			want:  "$x = 42\n",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "only a comment",
			input: "# hello\n",
			want:  "# hello\n",
		},
		{
			name:  "spacing",
			input: "$x=42\n",
			want:  "$x = 42\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := newFormatter(t)

			output, err := formatter.FormatData(context.TODO(), strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("func FormatData failed: %+v", err)
			}
			if string(output) != tt.want {
				t.Fatalf("unexpected output:\n%s", output)
			}
		})
	}
}

func TestFormatDataNilInput(t *testing.T) {
	formatter := newFormatter(t)

	if _, err := formatter.FormatData(context.TODO(), nil); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFormatDataParseError(t *testing.T) {
	formatter := newFormatter(t)

	if _, err := formatter.FormatData(context.TODO(), strings.NewReader("$x = = 42\n")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFormatPathRequiresAbsolutePath(t *testing.T) {
	formatter := newFormatter(t)

	if _, err := formatter.FormatPath(context.TODO(), "main.mcl"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFormatPathTrailingSlashMeansDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.mcl"), []byte("$x = 42\n"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}

	formatter := newFormatter(t)

	ok, err := formatter.FormatPath(context.TODO(), dir+"/")
	if err != nil {
		t.Fatalf("func FormatPath failed: %+v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}

	if _, err := formatter.FormatPath(context.TODO(), dir); err == nil {
		t.Fatalf("expected directory without trailing slash to be read as a file")
	}
}

func TestFormatFileWritesFinalNewline(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "main.mcl")
	if err := os.WriteFile(filename, []byte("$x = 42"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}

	formatter := newFormatter(t)

	ok, err := formatter.FormatFile(context.TODO(), filename)
	if err != nil {
		t.Fatalf("func FormatFile failed: %+v", err)
	}
	if ok {
		t.Fatalf("expected changed file")
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("func ReadFile failed: %+v", err)
	}
	if string(data) != "$x = 42\n" {
		t.Fatalf("unexpected file contents:\n%s", data)
	}
}

func TestFormatFileTestModeDoesNotWrite(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "main.mcl")
	if err := os.WriteFile(filename, []byte("$x = 42"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}

	formatter := newFormatter(t)
	formatter.Test = true

	ok, err := formatter.FormatFile(context.TODO(), filename)
	if err != nil {
		t.Fatalf("func FormatFile failed: %+v", err)
	}
	if ok {
		t.Fatalf("expected changed file")
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("func ReadFile failed: %+v", err)
	}
	if string(data) != "$x = 42" {
		t.Fatalf("unexpected file contents:\n%s", data)
	}
}

func TestFormatDirRecursesMCLFiles(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatalf("func Mkdir failed: %+v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.mcl"), []byte("$x = 42\n"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "other.mcl"), []byte("$y = 13\n"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "ignored.txt"), []byte("not mcl"), 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}

	formatter := newFormatter(t)

	ok, err := formatter.FormatDir(context.TODO(), dir)
	if err != nil {
		t.Fatalf("func FormatDir failed: %+v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}
}

// testFS is a minimal, read-only filesystem which fulfills the engine.ReadFS
// interface. Each instance is a distinct filesystem with its own root, which is
// what lets us check that we read each file from the right place.
type testFS struct {
	uri   string
	files map[string][]byte
}

// URI returns the unique handle for this filesystem.
func (obj *testFS) URI() string { return obj.uri }

// ReadFile returns the contents of the named file in this filesystem.
func (obj *testFS) ReadFile(name string) ([]byte, error) {
	data, exists := obj.files[name]
	if !exists {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func TestCheckFiles(t *testing.T) {
	// Both of these filesystems have their own /main.mcl file, which is
	// what happens when we import more than one embedded module.
	fs1 := &testFS{
		uri: "testfs:///one",
		files: map[string][]byte{
			"/main.mcl": []byte("$x = 42\n"),
		},
	}
	fs2 := &testFS{
		uri: "testfs:///two",
		files: map[string][]byte{
			"/main.mcl": []byte("$y=13\n"), // not formatted
		},
	}

	formatter := newFormatter(t)

	files := []*interfaces.SourceFile{
		{FS: fs1, Path: "/main.mcl"},
	}
	checkOK, err := formatter.CheckFiles(context.TODO(), files)
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if !checkOK {
		t.Fatalf("expected check ok")
	}

	files = append(files, &interfaces.SourceFile{FS: fs2, Path: "/main.mcl"})
	checkOK, err = formatter.CheckFiles(context.TODO(), files)
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if checkOK {
		t.Fatalf("expected check failure")
	}

	// A file which we can't read is an error, and not a check failure.
	files = []*interfaces.SourceFile{
		{FS: fs1, Path: "/missing.mcl"},
	}
	if _, err := formatter.CheckFiles(context.TODO(), files); err == nil {
		t.Fatalf("expected error for a missing file")
	}
}

// sourceFiles compiles the given mcl code as if it was the main file of a
// deploy, and it returns every source file that the result is built out of.
// This includes the files of any imported module, which is how we get our hands
// on a file that lives in a filesystem other than the one we started with.
func sourceFiles(t *testing.T, code []byte) []*interfaces.SourceFile {
	t.Helper()

	memFs := afero.NewMemMapFs()
	if err := afero.WriteFile(memFs, "/main.mcl", code, 0600); err != nil {
		t.Fatalf("func WriteFile failed: %+v", err)
	}
	fs := &util.AferoFs{Afero: &afero.Afero{Fs: memFs}}

	output, err := inputs.ParseInput("/main.mcl", fs)
	if err != nil {
		t.Fatalf("func ParseInput failed: %+v", err)
	}

	xast, err := parser.LexParse(bytes.NewReader(output.Main))
	if err != nil {
		t.Fatalf("func LexParse failed: %+v", err)
	}

	importGraph, err := pgraph.NewGraph("importGraph")
	if err != nil {
		t.Fatalf("func NewGraph failed: %+v", err)
	}
	importVertex := &pgraph.SelfVertex{
		Name:  "", // first node is the empty string
		Graph: importGraph,
	}
	importGraph.AddVertex(importVertex)

	data := &interfaces.Data{
		Fs:       output.FS,
		FsURI:    output.FS.URI(),
		Base:     output.Base,
		Files:    output.Files,
		Imports:  importVertex,
		Metadata: output.Metadata,
		Modules:  "/" + interfaces.ModuleDirectory,

		LexParser:       parser.LexParse,
		StrInterpolater: interpolate.StrInterpolate,

		Debug: testing.Verbose(), // set via the -test.v flag to `go test`
		Logf: func(format string, v ...interface{}) {
			t.Logf("ast: "+format, v...)
		},
	}
	if err := xast.Init(data); err != nil {
		t.Fatalf("func Init failed: %+v", err)
	}

	iast, err := xast.Interpolate()
	if err != nil {
		t.Fatalf("func Interpolate failed: %+v", err)
	}

	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		"hostname":   &ast.ExprStr{V: ""},             // not used
	}
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		t.Fatalf("func MergeExprMaps failed: %+v", err)
	}
	scope := &interfaces.Scope{
		Variables: variables,
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}
	if err := iast.SetScope(scope); err != nil {
		t.Fatalf("func SetScope failed: %+v", err)
	}

	programs, err := ast.CollectPrograms(iast)
	if err != nil {
		t.Fatalf("func CollectPrograms failed: %+v", err)
	}

	files := []*interfaces.SourceFile{}
	for _, program := range programs {
		files = append(files, program.File)
	}
	return files
}

// TestCheckFilesEmbeddedImport is the regression test for the bug where running
// `mgmt check lang` on any program which imports an embedded module failed with
// a "could not read: /main.mcl" error. That module is compiled into the binary
// and it has its own filesystem root, so its /main.mcl only exists there, and
// looking for that path on the local disk finds nothing. See the matching
// tests/embedded.mcl file for more information.
func TestCheckFilesEmbeddedImport(t *testing.T) {
	good, err := os.ReadFile(filepath.Join("tests/", "embedded.mcl"))
	if err != nil {
		t.Fatalf("could not read file: %+v", err)
	}
	bad, err := os.ReadFile(filepath.Join("tests/", "embedded.badmcl"))
	if err != nil {
		t.Fatalf("could not read file: %+v", err)
	}

	files := sourceFiles(t, good)
	if len(files) < 2 {
		t.Fatalf("expected the imported files too, got %d", len(files))
	}

	// The main file comes from the fs we made, everything else is embedded.
	embeds := 0
	for _, file := range files[1:] {
		if !strings.HasPrefix(file.URI(), embedded.Scheme+"://") {
			continue
		}
		embeds++
		// This is what used to fail: the path exists in the embedded fs,
		// but it does not exist on the local disk.
		if _, err := file.Source(); err != nil {
			t.Errorf("could not read embedded file %s: %+v", file.URI(), err)
		}
	}
	if embeds == 0 {
		t.Fatalf("no embedded files were collected")
	}

	formatter := newFormatter(t)
	formatter.Verbose = true // log the name of any file that isn't formatted

	// Everything here is formatted, including the embedded module, which we
	// can only know if we actually read it.
	checkOK, err := formatter.CheckFiles(context.TODO(), files)
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if !checkOK {
		t.Errorf("expected check ok")
	}

	// The same program, but with our main file no longer formatted.
	checkOK, err = formatter.CheckFiles(context.TODO(), sourceFiles(t, bad))
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if checkOK {
		t.Errorf("expected check failure")
	}
}

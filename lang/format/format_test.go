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

	"github.com/purpleidea/mgmt/lang/format"
	"github.com/purpleidea/mgmt/lang/format/astfmt"
	"github.com/purpleidea/mgmt/lang/parser"

	godiff "github.com/kylelemons/godebug/diff"
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
		Logf: func(format string, v ...any) {
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

		if before, ok := strings.CutSuffix(name, ".badmcl"); ok {
			t.Run(name, func(t *testing.T) {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatalf("could not read file: %+v", err)
				}

				goodName := before + ".mcl"
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

func TestCheckFiles(t *testing.T) {
	files := map[string][]byte{
		"/main.mcl":  []byte("$x = 42\n"),
		"/other.mcl": []byte("$y=13\n"), // not formatted
	}
	sourceFinder := func(filename string) ([]byte, error) {
		data, exists := files[filename]
		if !exists {
			return nil, os.ErrNotExist
		}
		return data, nil
	}

	formatter := newFormatter(t)

	checkOK, err := formatter.CheckFiles(context.TODO(), []string{"/main.mcl"}, sourceFinder)
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if !checkOK {
		t.Fatalf("expected check ok")
	}

	checkOK, err = formatter.CheckFiles(context.TODO(), []string{"/main.mcl", "/other.mcl"}, sourceFinder)
	if err != nil {
		t.Fatalf("func CheckFiles failed: %+v", err)
	}
	if checkOK {
		t.Fatalf("expected check failure")
	}
}

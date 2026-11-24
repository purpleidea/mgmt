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

package util

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
	"time"
)

// dirSyncTree returns each file under root with its contents, and each dir with
// a trailing slash and no contents.
func dirSyncTree(t *testing.T, root string) map[string]string {
	t.Helper()
	r, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}
	defer r.Close()
	fsys := r.FS()
	m := map[string]string{}
	err = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if d.IsDir() {
			m[p+"/"] = ""
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		m[p] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return m
}

// dirSyncMtimes returns the mtime of each file and dir under root.
func dirSyncMtimes(t *testing.T, root string) map[string]time.Time {
	t.Helper()
	m := map[string]time.Time{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		m[p] = info.ModTime()
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	return m
}

func TestSyncDirToDisk1(t *testing.T) {
	srcFs := NewIOFs(fstest.MapFS{
		"src/same":      &fstest.MapFile{Data: []byte("same"), Mode: 0644},
		"src/changed":   &fstest.MapFile{Data: []byte("new"), Mode: 0644},
		"src/new":       &fstest.MapFile{Data: []byte("new"), Mode: 0644},
		"src/wasdir":    &fstest.MapFile{Data: []byte("file"), Mode: 0644},
		"src/wasfile":   &fstest.MapFile{Mode: fs.ModeDir | 0755},
		"src/wasfile/x": &fstest.MapFile{Data: []byte("x"), Mode: 0644},
		"src/sub":       &fstest.MapFile{Mode: fs.ModeDir | 0755},
		"src/sub/y":     &fstest.MapFile{Data: []byte("y"), Mode: 0644},
	})

	dst := t.TempDir()
	for _, x := range []string{"wasdir", "extradir"} {
		if err := os.Mkdir(filepath.Join(dst, x), 0750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	setup := map[string]string{
		"same":       "same",
		"changed":    "old",
		"extra":      "extra",
		"extradir/z": "z",
		"wasdir/w":   "w",
		"wasfile":    "file",
	}
	for k, v := range setup {
		if err := os.WriteFile(filepath.Join(dst, k), []byte(v), 0600); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}
	old := time.Unix(1000000000, 0)
	same := filepath.Join(dst, "same")
	if err := os.Chtimes(same, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := SyncDirToDisk(srcFs, "/src/", dst); err != nil {
		t.Fatalf("sync: %v", err)
	}

	expected := map[string]string{
		"same":      "same",
		"changed":   "new",
		"new":       "new",
		"wasdir":    "file",
		"wasfile/":  "",
		"wasfile/x": "x",
		"sub/":      "",
		"sub/y":     "y",
	}
	if tree := dirSyncTree(t, dst); !reflect.DeepEqual(tree, expected) {
		t.Errorf("tree: got %v, expected %v", tree, expected)
	}
	info, err := os.Stat(same)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("same file was written")
	}

	// A second sync has nothing to do, so it mustn't touch anything.
	mtimes := dirSyncMtimes(t, dst)
	if err := SyncDirToDisk(srcFs, "/src/", dst); err != nil {
		t.Fatalf("sync again: %v", err)
	}
	if tree := dirSyncTree(t, dst); !reflect.DeepEqual(tree, expected) {
		t.Errorf("tree again: got %v, expected %v", tree, expected)
	}
	if m := dirSyncMtimes(t, dst); !reflect.DeepEqual(m, mtimes) {
		t.Errorf("sync again changed an mtime")
	}
}

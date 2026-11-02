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

package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
)

// testFileEdit returns a FileEdit API to test with.
func testFileEdit(t *testing.T) *FileEditImpl {
	t.Helper()
	obj := &FileEditImpl{}
	obj.Init(&FileEditInit{
		Logf: func(format string, v ...interface{}) { t.Logf("fileedit: "+format, v...) },
	})
	return obj
}

// testSet returns an edit function which sets the contents to this string.
func testSet(s string) func(*string) (*string, error) {
	return func(*string) (*string, error) {
		return &s, nil
	}
}

func TestFileEdit(t *testing.T) {
	ctx := context.Background()
	obj := testFileEdit(t)
	path := filepath.Join(t.TempDir(), "edit.json")

	if err := obj.FileEdit(ctx, "relative.json", testSet("{}")); err == nil {
		t.Errorf("func FileEdit: expected an error for a relative path")
	}

	// A missing file comes in as nil, and returning nil keeps it missing.
	if err := obj.FileEdit(ctx, path, func(value *string) (*string, error) {
		if value != nil {
			return nil, fmt.Errorf("unexpected value: %s", *value)
		}
		return nil, nil
	}); err != nil {
		t.Fatalf("func FileEdit: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file exists, or other error: %v", err)
	}

	// Create it, with private permissions.
	if err := obj.FileEdit(ctx, path, testSet("hello\n")); err != nil {
		t.Fatalf("func FileEdit: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("func Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0600); got != want {
		t.Errorf("new file mode: got %o, want %o", got, want)
	}

	// An existing file keeps its mode.
	if err := os.Chmod(path, 0640); err != nil { //nolint:gosec // G302: We're testing Chmod functionality
		t.Fatalf("func Chmod: %v", err)
	}
	if err := obj.FileEdit(ctx, path, func(value *string) (*string, error) {
		if value == nil || *value != "hello\n" {
			return nil, fmt.Errorf("unexpected value: %v", value)
		}
		s := "world\n"
		return &s, nil
	}); err != nil {
		t.Fatalf("func FileEdit: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	if string(data) != "world\n" {
		t.Errorf("unexpected contents: %q", data)
	}
	if info, err = os.Stat(path); err != nil {
		t.Fatalf("func Stat: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0640); got != want {
		t.Errorf("edited file mode: got %o, want %o", got, want)
	}

	// An error from the edit function doesn't change anything.
	if err := obj.FileEdit(ctx, path, func(*string) (*string, error) {
		return nil, fmt.Errorf("nope")
	}); err == nil {
		t.Errorf("func FileEdit: expected an error")
	}
	if data, err = os.ReadFile(path); err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	if string(data) != "world\n" {
		t.Errorf("unexpected contents: %q", data)
	}

	// Returning nil removes it.
	if err := obj.FileEdit(ctx, path, func(*string) (*string, error) {
		return nil, nil
	}); err != nil {
		t.Fatalf("func FileEdit: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file exists, or other error: %v", err)
	}

	// Nothing but the file was ever left in the dir.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("func ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("unexpected files in dir: %v", entries)
	}
}

func TestFileEditNotRegular(t *testing.T) {
	obj := testFileEdit(t)
	if err := obj.FileEdit(context.Background(), t.TempDir(), testSet("{}")); err == nil {
		t.Errorf("func FileEdit: expected an error for a dir")
	}
}

func TestFileEditCancelled(t *testing.T) {
	obj := testFileEdit(t)
	path := filepath.Join(t.TempDir(), "edit.json")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := obj.FileEdit(ctx, path, testSet("{}")); err != context.Canceled {
		t.Errorf("func FileEdit: unexpected error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file exists, or other error: %v", err)
	}
}

// TestFileEditConcurrent runs many edits at once, and none of them can be lost.
// It also edits the same file through a different spelling of its path. Once
// they're done, we must have forgotten every lock.
func TestFileEditConcurrent(t *testing.T) {
	ctx := context.Background()
	obj := testFileEdit(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "counter")
	paths := []string{path, dir + "//./counter"}
	count := 50

	wg := &sync.WaitGroup{}
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- obj.FileEdit(ctx, paths[i%len(paths)], func(value *string) (*string, error) {
				n := 0
				if value != nil {
					var err error
					if n, err = strconv.Atoi(*value); err != nil {
						return nil, err
					}
				}
				s := strconv.Itoa(n + 1)
				return &s, nil
			})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("func FileEdit: %v", err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("func ReadFile: %v", err)
	}
	if string(data) != strconv.Itoa(count) {
		t.Errorf("unexpected contents: %q", data)
	}

	obj.mutex.Lock()
	defer obj.mutex.Unlock()
	if l := len(obj.locks); l != 0 {
		t.Errorf("still have %d locks", l)
	}
}

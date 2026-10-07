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

package atomicfile

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"testing"
)

func TestWrites(t *testing.T) {
	workdir, err := os.MkdirTemp("", "atomic-file")
	if err != nil {
		t.Errorf("test bug: failed to create a temporary directory to use with testing: %s", err)
		return
	}

	defer os.Remove(workdir)

	filename := path.Join(workdir, "hello-world")

	_, err = os.Stat(filename)
	if err == nil {
		t.Errorf("test bug: File shouldn't exist before we commit it")
		return
	}

	// Test new file creation
	check(t, filename, "Hello world!")

	_, err = os.Stat(filename)
	if err != nil {
		t.Errorf("test bug: File should exist at this point: %s", err)
		return
	}

	// Test atomic file replacement
	check(t, filename, "Put on your fancy pants!")
	os.Remove(filename)

}

func TestMetadata(t *testing.T) {
	workdir, err := os.MkdirTemp("", "atomic-file")
	if err != nil {
		t.Errorf("during setup, failed to create a temporary directory to use with testing: %s", err)
		return
	}
	defer os.Remove(workdir)

	filename := path.Join(workdir, "hello-world")

	checkStat(t, filename,
		func(f *AtomicFile) error {
			return f.Chmod(0654)
		},
		func(i fs.FileInfo) error {
			if i.Mode() != 0654 {
				return fmt.Errorf("file mode expected to be %0o, but is %0o", 0654, i.Mode())
			}
			return nil
		},
	)
}

func TestWithoutCommit(t *testing.T) {
	workdir, err := os.MkdirTemp("", "atomic-file")
	if err != nil {
		t.Errorf("during setup, failed to create a temporary directory to use with testing: %s", err)
		return
	}

	defer os.Remove(workdir)

	filename := path.Join(workdir, "hello-world")

	_, err = os.Stat(filename)
	if err == nil {
		t.Errorf("test Bug: File shouldn't exist before we commit it")
		return
	}

	f, err := New(filename)

	if err != nil {
		t.Errorf("bug: New(%s) failed unexpectedly: %s", filename, err)
		return
	}

	input_bytes := []byte("Hello world")

	n, err := f.Write(input_bytes)
	if err != nil {
		t.Errorf("test bug: Write() expected to write %d bytes, but only %d were written: %s", len(input_bytes), n, err)
		return
	}

	_, err = os.ReadFile(filename)

	if err == nil {
		// An error is expected, this file should not exist because we did not Commit()
		t.Errorf("os.ReadFile should fail when an AtomicFile has not been Commit()'d.")
		return
	}

	err = f.Close()
	if err != nil {
		t.Errorf("bug: Close() failed: %s", err)
		return
	}

	// Assert the temporary file path doesn't exist either.
	files, err := os.ReadDir(path.Dir(f.path))
	if err != nil {
		t.Errorf("test bug: could not read the test's temporary directory")
		return
	}
	if len(files) > 1 {
		t.Errorf("bug: temporary file should not exist after Close()")
		return
	}
}

func FuzzMetadata(f *testing.F) {
	workdir, err := os.MkdirTemp("", "atomic-file")
	if err != nil {
		f.Errorf("during setup, failed to create a temporary directory to use with testing: %s", err)
		return
	}
	defer os.Remove(workdir)

	filename := path.Join(workdir, "hello-world")
	f.Add(int32(0755), "hello-world")

	const minimal = 0400
	// Go's fuzzer has no concept(?) of os.FileMode, so in order to fuzz it,
	f.Fuzz(func(t *testing.T, perms int32, _ string) {
		// We have to make sure the permissions are valid.
		// Permissions must have owner read, for example.
		mode := os.FileMode(perms&0777 | minimal)

		checkStat(t, filename,
			func(f *AtomicFile) error {
				//t.Logf("Chmod %#o", mode)
				return f.Chmod(mode)
			},
			func(i fs.FileInfo) error {
				if i.Mode() != mode {
					return fmt.Errorf("file mode expected to be %0o, but is %0o", mode, i.Mode())
				}
				return nil
			},
		)
	})
}

func check(t *testing.T, filename string, input string) {
	f, err := New(filename)

	if err != nil {
		t.Errorf("call to New(%s) failed unexpectedly: %s", filename, err)
		return
	}

	input_bytes := []byte(input)

	n, err := f.Write(input_bytes)
	//if n < len(input_bytes) {
	if err != nil {
		t.Errorf("call to Write() expected to write %d bytes, but only %d were written: %s", len(input_bytes), n, err)
		return
	}

	// Check that the target file doesn't already have our contents
	// This check assumes the tet suite isn't writing the same contents to an existing file.
	data, err := os.ReadFile(filename)
	if err != nil {
		// It's ok if reading failed, maybe the file doesn't exist.
	} else if bytes.Equal(data, input_bytes) {
		t.Errorf("bug: before Commit(), the file contents should not have been written.")
		return
	}

	err = f.Commit()
	if err != nil {
		t.Errorf("bug: Commit() failed: %s", err)
		return
	}

	data, err = os.ReadFile(filename)

	if err != nil {
		t.Errorf("bug: error reading file after commit: %s", err)
		return
	}

	if !bytes.Equal(data, input_bytes) {
		t.Errorf("bug: file contents did not match what was written %v vs %v", string(data), string(input_bytes))
		return
	}

	// Assert the temporary file path doesn't exist either.
	files, err := os.ReadDir(path.Dir(f.path))
	if err != nil {
		t.Errorf("test bug: could not read the test's temporary directory")
		return
	}
	if len(files) > 1 {
		t.Errorf("bug: temporary file should not exist after Close() - %s vs %s", f.Name(), f.path)
		return
	}

}

func checkStat(t *testing.T, filename string, operation func(*AtomicFile) error, validate func(fs.FileInfo) error) {
	f, err := New(filename)
	if err != nil {
		t.Errorf("call to New(%s) failed unexpectedly: %s", filename, err)
		return
	}

	err = operation(f)
	if err != nil {
		t.Errorf("test bug: checkStat operation func returned error: %s", err)
		return
	}

	err = f.Commit()
	if err != nil {
		t.Errorf("bug: call to Commit() failed for file %s: %s", f.path, err)
		return
	}

	stat, err := os.Stat(filename)
	if err != nil {
		t.Errorf("test bug: os.Stat() failed: %s", err)
		return
	}

	err = validate(stat)
	if err != nil {
		t.Errorf("bug: checkStat validation failed: %s", err)
		return
	}
}

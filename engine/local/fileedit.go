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

package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// FileEditInit are the init values that the FileEdit API needs to work
// correctly.
type FileEditInit struct {
	Debug bool
	Logf  func(format string, v ...interface{})
}

// FileEditImpl is the implementation for the FileEdit API's. The API's are the
// collection of public methods that exist on this struct. It edits whole files
// on this machine atomically, and it serializes the edits to each path, so the
// resources and functions in this process which share a file never lose any of
// each other's changes, without having to coordinate that among themselves. It
// can't stop some other process from editing that same file at the same time.
type FileEditImpl struct {
	init  *FileEditInit
	mutex *sync.Mutex
	locks map[string]*fileEditLock // only for the paths being edited now
}

// Init runs some initialization code for the FileEdit API.
func (obj *FileEditImpl) Init(init *FileEditInit) {
	obj.init = init
	obj.mutex = &sync.Mutex{}
	obj.locks = make(map[string]*fileEditLock)
}

// FileEdit atomically edits the file at an absolute path, with a
// compare-and-swap methodology. This can either create, delete, or change an
// existing file. The current file contents are passed in to the function. The
// nil value is sent if no file currently exists. The function is then run, and
// assuming nothing goes wrong, the returned value is used during the edit. You
// may return nil to express that you wish to delete the file. The function must
// be pure (it must only depend on its input) and it may be run multiple times
// as needed. This mechanism lets multiple writers (within this process)
// serialize their writes, so that none are lost. A new file is made with 0600
// permissions, and an existing one keeps its mode and its owner. The contents
// are written to a temporary file beside it, which is renamed into place, so a
// reader never sees a partial file. Two paths which reach the same file through
// a symlink are not serialized with each other.
func (obj *FileEditImpl) FileEdit(ctx context.Context, path string, fn func(*string) (*string, error)) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute")
	}
	path = filepath.Clean(path)

	unlock := obj.lock(path)
	defer unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		return errwrap.Wrapf(err, "could not stat file")
	}
	if err != nil {
		info = nil // it doesn't exist
	}
	if info != nil && !info.Mode().IsRegular() {
		return fmt.Errorf("the path is not a regular file")
	}

	var value *string // nil if it doesn't exist
	if info != nil {
		b, err := os.ReadFile(path)
		if err != nil {
			return errwrap.Wrapf(err, "could not read file")
		}
		s := string(b)
		value = &s
	}

	data, err := fn(value)
	if err != nil {
		return err
	}
	if value == nil && data == nil {
		return nil // already absent
	}
	if value != nil && data != nil && *value == *data {
		return nil // already set
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if data == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errwrap.Wrapf(err, "could not remove file")
		}
		return syncDir(filepath.Dir(path))
	}
	return writeFileAtomic(ctx, path, info, []byte(*data))
}

// lock takes the lock for this path, and returns the function to release it.
func (obj *FileEditImpl) lock(path string) func() {
	obj.mutex.Lock()
	l, exists := obj.locks[path]
	if !exists {
		l = &fileEditLock{
			mutex: &sync.Mutex{},
		}
		obj.locks[path] = l
	}
	l.refs++ // before we wait, so that nobody can forget it while we do
	obj.mutex.Unlock()

	l.mutex.Lock()

	return func() {
		l.mutex.Unlock()

		obj.mutex.Lock()
		defer obj.mutex.Unlock()
		l.refs--
		if l.refs == 0 {
			delete(obj.locks, path)
		}
	}
}

// fileEditLock is the lock for one path, which counts its users, so that we can
// forget it once nobody is using it.
type fileEditLock struct {
	mutex *sync.Mutex
	refs  int
}

// writeFileAtomic replaces the file at this path with the data. If we have the
// info of an existing file there, then we keep its mode and its owner.
func writeFileAtomic(ctx context.Context, path string, info os.FileInfo, data []byte) (reterr error) {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".") // made with 0600
	if err != nil {
		return errwrap.Wrapf(err, "could not create temporary file")
	}
	tmp := file.Name()
	closed := false
	defer func() {
		if !closed {
			if err := file.Close(); err != nil && reterr == nil {
				reterr = errwrap.Wrapf(err, "could not close temporary file")
			}
		}
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) && reterr == nil {
			reterr = errwrap.Wrapf(err, "could not remove temporary file")
		}
	}()

	if info != nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			if err := file.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
				return errwrap.Wrapf(err, "could not preserve file ownership")
			}
		}
		if err := file.Chmod(info.Mode().Perm()); err != nil {
			return errwrap.Wrapf(err, "could not preserve file mode")
		}
	}
	if _, err := file.Write(data); err != nil {
		return errwrap.Wrapf(err, "could not write temporary file")
	}
	if err := file.Sync(); err != nil {
		return errwrap.Wrapf(err, "could not sync temporary file")
	}
	err = file.Close()
	closed = true
	if err != nil {
		return errwrap.Wrapf(err, "could not close temporary file")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return errwrap.Wrapf(err, "could not replace file")
	}

	return syncDir(filepath.Dir(path))
}

// syncDir syncs a directory, so that a change to its entries is durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return errwrap.Wrapf(err, "could not open directory")
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return errwrap.Wrapf(err, "could not sync directory")
	}
	return nil
}

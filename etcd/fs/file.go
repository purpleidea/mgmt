// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package fs

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	etcdutil "github.com/coreos/etcd/clientv3/clientv3util"
)

func init() {
	gob.Register(&File{})
}

// File represents a file node. This is the node of our tree structure. This is
// not thread safe, and you can have at most one open file handle at a time.
type File struct {
	// FIXME: add a rwmutex to make this thread safe
	fs *Fs // pointer to file system

	Path    string // relative path to file, trailing slash if it's a directory
	Mode    os.FileMode
	ModTime time.Time
	//Size int64 // XXX: cache the size to avoid full file downloads for stat!

	Children []*File // dir's use this
	Hash     string  // string not []byte so it's readable, matches data

	data      []byte // cache of the data. private so it doesn't get encoded
	cursor    int64
	dirCursor int64

	readOnly bool // is the file read-only?
	closed   bool // is file closed?
}

// path returns the expected path to the actual file in etcd.
func (obj *File) path() string {
	// keys are prefixed with the hash-type eg: {sha256} to allow different
	// superblocks to share the same data prefix even with different hashes
	return fmt.Sprintf("%s/{%s}%s", obj.fs.sb.DataPrefix, obj.fs.Hash, obj.Hash)
}

// cache downloads the file contents from etcd and stores them in our cache.
func (obj *File) cache() error {
	if obj.Mode.IsDir() {
		return nil
	}

	h, err := obj.fs.hash(obj.data) // update hash
	if err != nil {
		return err
	}
	if h == obj.Hash { // we already have the correct data cached
		return nil
	}

	p := obj.path() // get file data from this path in etcd

	result, err := obj.fs.get(p) // download the file...
	if err != nil {
		return err
	}
	if result == nil || len(result) == 0 { // nothing found
		return err
	}
	data, exists := result[p]
	if !exists {
		return fmt.Errorf("could not find data") // programming error?
	}
	obj.data = data // save
	return nil
}

// findNode is the "in array" equivalent for searching through a dir's children.
// You must *not* specify an absolute path as the search string, but rather you
// should specify the name. To search for something name "bar" inside a dir
// named "/tmp/foo/", you just pass in "bar", not "/tmp/foo/bar".
func (obj *File) findNode(name string) (*File, bool) {
	for _, node := range obj.Children {
		if name == node.Path {
			return node, true // found
		}
	}
	return nil, false // not found
}

func fileCreate(fs *Fs, name string) (*File, error) {
	if name == "" {
		return nil, fmt.Errorf("invalid input path")
	}
	if !strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("invalid input path (not absolute)")
	}
	cleanPath := path.Clean(name) // remove possible trailing slashes

	// try to add node to tree by first finding the parent node
	parentPath, filePath := path.Split(cleanPath) // looking for this

	node, err := fs.find(parentPath)
	if err != nil { // might be ErrNotExist
		return nil, err
	}

	fi, err := node.Stat()
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() { // is the parent a suitable home?
		return nil, &os.PathError{Op: "create", Path: name, Err: syscall.ENOTDIR}
	}

	f, exists := node.findNode(filePath) // does file already exist inside?
	if exists {                          // already exists, overwrite!
		if err := f.Truncate(0); err != nil {
			return nil, err
		}
		return f, nil
	}

	data := []byte("")      // empty file contents
	h, err := fs.hash(data) // TODO: use memoized value?
	if err != nil {
		return &File{}, err // TODO: nil instead?
	}

	f = &File{
		fs:   fs,
		Path: filePath, // the relative path chunk (not incl. dir name)
		Hash: h,
		data: data,
	}

	// add to parent
	node.Children = append(node.Children, f)

	// push new file up if not on server, and then push up the metadata
	if err := f.Sync(); err != nil {
		return f, err // TODO: ok to return the file so user can run sync?
	}

	return f, nil
}

func fileOpen(fs *Fs, name string) (*File, error) {
	if name == "" {
		return nil, fmt.Errorf("invalid input path")
	}
	if !strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("invalid input path (not absolute)")
	}
	cleanPath := path.Clean(name) // remove possible trailing slashes

	node, err := fs.find(cleanPath)
	if err != nil { // might be ErrNotExist
		return &File{}, err // TODO: nil instead?
	}

	// download file contents into obj.data
	if err := node.cache(); err != nil {
		return &File{}, err // TODO: nil instead?
	}

	//fi, err := node.Stat()
	//if err != nil {
	//	return nil, err
	//}
	//if fi.IsDir() { // can we open a directory? - yes we can apparently
	//	return nil, fmt.Errorf("file is a directory")
	//}

	node.readOnly = true // as per docs, fileOpen opens files as read-only
	node.closed = false  // as per docs, fileOpen opens files as read-only

	return node, nil
}

// Close closes the file handle. This will try and run Sync automatically.
func (obj *File) Close() error {
	if !obj.readOnly {
		obj.ModTime = time.Now()
	}

	if err := obj.Sync(); err != nil {
		return err
	}

	// FIXME: there is a big implementation mistake between the metadata
	// node and the file handle, since they're currently sharing a struct!

	// invalidate all of the fields
	//obj.fs = nil

	//obj.Path = ""
	//obj.Mode = os.FileMode(0)
	//obj.ModTime = time.Time{}

	//obj.Children = nil
	//obj.Hash = ""

	//obj.data = nil
	obj.cursor = 0
	obj.readOnly = false

	obj.closed = true
	return nil
}

// Name returns the path of the file.
func (obj *File) Name() string {
	return obj.Path
}

// Stat returns some information about the file.
func (obj *File) Stat() (os.FileInfo, error) {
	// download file contents into obj.data
	if err := obj.cache(); err != nil { // needed so Size() works correctly
		return nil, err
	}
	return &FileInfo{ // everything is actually stored in the main file node
		file: obj,
	}, nil
}

// Sync flushes the file contents to the server and calls the filesystem
// metadata sync as well.
// FIXME: instead of a txn, run a get and then a put in two separate stages. if
// the get already found the data up there, then we don't need to push it all in
// the put phase. with the txn it is always all sent up even if the put is never
// needed. the get should just be a "key exists" test, and not a download of the
// whole file. if we *do* do the download, we can byte-by-byte check for hash
// collisions and panic if we find one :)
func (obj *File) Sync() error {
	if obj.closed {
		return ErrFileClosed
	}

	p := obj.path() // store file data at this path in etcd

	//cmp := etcd.Compare(etcd.Version(p), "=", 0) // KeyMissing
	cmp := etcdutil.KeyMissing(p)
	op := etcd.OpPut(p, string(obj.data)) // this pushes contents to server

	// it's important to do this in one transaction, and atomically, because
	// this way, we only generate one watch event, and only when it's needed
	result, err := obj.fs.txn([]etcd.Cmp{cmp}, []etcd.Op{op}, nil)
	if err != nil {
		return errwrap.Wrapf(err, "sync error with: %s (%s)", obj.Path, p)
	}
	if !result.Succeeded {
		if obj.fs.Debug {
			obj.fs.Logf("debug: data already exists in storage")
		}
	}

	if err := obj.fs.sync(); err != nil { // push metadata up to server
		return err
	}
	return nil
}

// Truncate trims the file to the requested size. Since our file system can only
// read and write data, but never edit existing data blocks, doing this will not
// cause more space to be available.
func (obj *File) Truncate(size int64) error {
	if obj.closed {
		return ErrFileClosed
	}
	if obj.readOnly {
		return &os.PathError{Op: "truncate", Path: obj.Path, Err: ErrFileReadOnly}
	}
	if size < 0 {
		return ErrOutOfRange
	}

	if size > 0 { // if size == 0, we don't need to run cache!
		// download file contents into obj.data
		if err := obj.cache(); err != nil {
			return err
		}
	}

	if size > int64(len(obj.data)) {
		diff := size - int64(len(obj.data))
		obj.data = append(obj.data, bytes.Repeat([]byte{00}, int(diff))...)
	} else {
		obj.data = obj.data[0:size]
	}

	h, err := obj.fs.hash(obj.data) // update hash
	if err != nil {
		return err
	}
	obj.Hash = h
	obj.ModTime = time.Now()

	// this pushes the new data and metadata up to etcd
	return obj.Sync()
}

// Read reads up to len(b) bytes from the File. It returns the number of bytes
// read and any error encountered. At end of file, Read returns 0, io.EOF.
// NOTE: This reads into the byte input. It's a side effect!
func (obj *File) Read(b []byte) (n int, err error) {
	if obj.closed {
		return 0, ErrFileClosed
	}
	if obj.Mode.IsDir() {
		return 0, fmt.Errorf("file is a directory")
	}

	// download file contents into obj.data
	if err := obj.cache(); err != nil {
		return 0, err // TODO: -1 ?
	}

	// TODO: can we optimize by reading just the length from etcd, and also
	// by only downloading the data range we're interested in?
	if len(b) > 0 && int(obj.cursor) == len(obj.data) {
		return 0, io.EOF
	}
	if len(obj.data)-int(obj.cursor) >= len(b) {
		n = len(b)
	} else {
		n = len(obj.data) - int(obj.cursor)
	}
	copy(b, obj.data[obj.cursor:obj.cursor+int64(n)]) // store into input b
	obj.cursor = obj.cursor + int64(n)                // update cursor

	return
}

// ReadAt reads len(b) bytes from the File starting at byte offset off. It
// returns the number of bytes read and the error, if any. ReadAt always returns
// a non-nil error when n < len(b). At end of file, that error is io.EOF.
func (obj *File) ReadAt(b []byte, off int64) (n int, err error) {
	obj.cursor = off
	return obj.Read(b)
}

// Readdir lists the contents of the directory and returns a list of file info
// objects for each entry.
func (obj *File) Readdir(count int) ([]os.FileInfo, error) {
	if !obj.Mode.IsDir() {
		return nil, &os.PathError{Op: "readdir", Path: obj.Name(), Err: syscall.ENOTDIR}
	}

	children := obj.Children[obj.dirCursor:] // available children to output
	var l = int64(len(children))             // initially assume to return them all
	var err error

	// for count > 0, if we return the last entry, also return io.EOF
	if count > 0 {
		l = int64(count) // initial assumption
		if c := len(children); count >= c {
			l = int64(c)
			err = io.EOF // this result includes the last dir entry
		}
	}
	obj.dirCursor += l // store our progress

	output := make([]os.FileInfo, l)
	// TODO: should this be sorted by "directory order" what does that mean?
	// from `man 3 readdir`: "unlikely that the names will be sorted"
	for i := range output {
		output[i] = &FileInfo{
			file: children[i],
		}
	}

	// we're seen the whole directory, so reset the cursor
	if err == io.EOF || count <= 0 {
		obj.dirCursor = 0 // TODO: is it okay to reset the cursor?
	}

	return output, err
}

// Readdirnames returns a list of name is the current file handle's directory.
// TODO: this implementation shares the dirCursor with Readdir, is this okay?
// TODO: should Readdirnames even use a dirCursor at all?
func (obj *File) Readdirnames(n int) (names []string, _ error) {
	fis, err := obj.Readdir(n)
	if fis != nil {
		for i, x := range fis {
			if x != nil {
				names = append(names, fis[i].Name())
			}
		}
	}
	return names, err
}

// Seek sets the offset for the next Read or Write on file to offset,
// interpreted according to whence: 0 means relative to the origin of the file,
// 1 means relative to the current offset, and 2 means relative to the end. It
// returns the new offset and an error, if any. The behavior of Seek on a file
// opened with O_APPEND is not specified.
func (obj *File) Seek(offset int64, whence int) (int64, error) {
	if obj.closed {
		return 0, ErrFileClosed
	}

	switch whence {
	case io.SeekStart: // 0
		obj.cursor = offset
	case io.SeekCurrent: // 1
		obj.cursor += offset
	case io.SeekEnd: // 2
		// download file contents into obj.data
		if err := obj.cache(); err != nil {
			return 0, err // TODO: -1 ?
		}
		obj.cursor = int64(len(obj.data)) + offset
	}
	return obj.cursor, nil
}

// Write writes to the given file.
func (obj *File) Write(b []byte) (n int, err error) {
	if obj.closed {
		return 0, ErrFileClosed
	}
	if obj.readOnly {
		return 0, &os.PathError{Op: "write", Path: obj.Path, Err: ErrFileReadOnly}
	}

	// download file contents into obj.data
	if err := obj.cache(); err != nil {
		return 0, err // TODO: -1 ?
	}

	// calculate the write
	n = len(b)
	cur := obj.cursor
	diff := cur - int64(len(obj.data))

	var tail []byte
	if n+int(cur) < len(obj.data) {
		tail = obj.data[n+int(cur):]
	}

	if diff > 0 {
		obj.data = append(bytes.Repeat([]byte{00}, int(diff)), b...)
		obj.data = append(obj.data, tail...)
	} else {
		obj.data = append(obj.data[:cur], b...)
		obj.data = append(obj.data, tail...)
	}

	h, err := obj.fs.hash(obj.data) // update hash
	if err != nil {
		return 0, err // TODO: -1 ?
	}
	obj.Hash = h
	obj.ModTime = time.Now()

	// this pushes the new data and metadata up to etcd
	if err := obj.Sync(); err != nil {
		return 0, err // TODO: -1 ?
	}

	obj.cursor = int64(len(obj.data))
	return
}

// WriteAt writes into the given file at a certain offset.
func (obj *File) WriteAt(b []byte, off int64) (n int, err error) {
	obj.cursor = off
	return obj.Write(b)
}

// WriteString writes a string to the file.
func (obj *File) WriteString(s string) (n int, err error) {
	return obj.Write([]byte(s))
}

// FileInfo is a struct which provides some information about a file handle.
type FileInfo struct {
	file *File // anonymous pointer to the actual file
}

// Name returns the base name of the file.
func (obj *FileInfo) Name() string {
	return obj.file.Name()
}

// Size returns the length in bytes.
func (obj *FileInfo) Size() int64 {
	return int64(len(obj.file.data))
}

// Mode returns the file mode bits.
func (obj *FileInfo) Mode() os.FileMode {
	return obj.file.Mode
}

// ModTime returns the modification time.
func (obj *FileInfo) ModTime() time.Time {
	return obj.file.ModTime
}

// IsDir is an abbreviation for Mode().IsDir().
func (obj *FileInfo) IsDir() bool {
	//return obj.file.Mode&os.ModeDir != 0
	return obj.file.Mode.IsDir()
}

// Sys returns the underlying data source (can return nil).
func (obj *FileInfo) Sys() interface{} {
	return nil // TODO: should we do something better?
	//return obj.file.fs // TODO: would this work?
}

// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

// Package fs implements a very simple and limited file system on top of etcd.
package fs

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/util/errwrap"

	etcd "github.com/coreos/etcd/clientv3" // "clientv3"
	rpctypes "github.com/coreos/etcd/etcdserver/api/v3rpc/rpctypes"
	"github.com/spf13/afero"
	context "golang.org/x/net/context"
)

func init() {
	gob.Register(&superBlock{})
}

const (
	// EtcdTimeout is the timeout to wait before erroring.
	EtcdTimeout = 5 * time.Second // FIXME: chosen arbitrarily
	// DefaultDataPrefix is the default path for data storage in etcd.
	DefaultDataPrefix = "/_etcdfs/data"
	// DefaultHash is the default hashing algorithm to use.
	DefaultHash = "sha256"
	// PathSeparator is the path separator to use on this filesystem.
	PathSeparator = os.PathSeparator // usually the slash character
)

// TODO: https://dave.cheney.net/2016/04/07/constant-errors
var (
	IsPathSeparator = os.IsPathSeparator

	// ErrNotImplemented is returned when something is not implemented by design.
	ErrNotImplemented = errors.New("not implemented")

	// ErrExist is returned when requested path already exists.
	ErrExist = os.ErrExist

	// ErrNotExist is returned when we can't find the requested path.
	ErrNotExist = os.ErrNotExist

	ErrFileClosed   = errors.New("File is closed")
	ErrFileReadOnly = errors.New("File handle is read only")
	ErrOutOfRange   = errors.New("Out of range")
)

// Fs is a specialized afero.Fs implementation for etcd. It implements a small
// subset of the features, and has some special properties. In particular, file
// data is stored with it's unique reference being a hash of the data. In this
// way, you cannot actually edit a file, but rather you create a new one, and
// update the metadata pointer to point to the new blob. This might seem slow,
// but it has the unique advantage of being relatively straight forward to
// implement, and repeated uploads of the same file cost almost nothing. Since
// etcd isn't meant for large file systems, this fits the desired use case.
// This implementation is designed to have a single writer for each superblock,
// but as many readers as you like.
// FIXME: this is not currently thread-safe, nor is it clear if it needs to be.
// XXX: we probably aren't updating the modification time everywhere we should!
// XXX: because we never delete data blocks, we need to occasionally "vacuum".
// XXX: this is harder because we need to list of *all* metadata paths, if we
// want them to be able to share storage backends. (we do)
type Fs struct {
	Client *etcd.Client

	Metadata string // location of "superblock" for this filesystem

	DataPrefix string // prefix of data storage (no trailing slashes)
	Hash       string // eg: sha256

	Debug bool

	sb      *superBlock
	mounted bool
}

// superBlock is the metadata structure of everything stored outside of the data
// section in etcd. Its fields need to be exported or they won't get marshalled.
type superBlock struct {
	DataPrefix string // prefix of data storage
	Hash       string // hashing algorithm used

	Tree *File // filesystem tree
}

// NewEtcdFs creates a new filesystem handle on an etcd client connection. You
// must specify the metadata string that you wish to use.
func NewEtcdFs(client *etcd.Client, metadata string) afero.Fs {
	return &Fs{
		Client:   client,
		Metadata: metadata,
	}
}

// get a number of values from etcd.
func (obj *Fs) get(path string, opts ...etcd.OpOption) (map[string][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdTimeout)
	resp, err := obj.Client.Get(ctx, path, opts...)
	cancel()
	if err != nil || resp == nil {
		return nil, err
	}

	// TODO: write a resp.ToMap() function on https://godoc.org/github.com/coreos/etcd/etcdserver/etcdserverpb#RangeResponse
	result := make(map[string][]byte) // formerly: map[string][]byte
	for _, x := range resp.Kvs {
		result[string(x.Key)] = x.Value // formerly: bytes.NewBuffer(x.Value).String()
	}

	return result, nil
}

// put a value into etcd.
func (obj *Fs) put(path string, data []byte, opts ...etcd.OpOption) error {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdTimeout)
	_, err := obj.Client.Put(ctx, path, string(data), opts...) // TODO: obj.Client.KV ?
	cancel()
	if err != nil {
		switch err {
		case context.Canceled:
			return errwrap.Wrapf(err, "ctx canceled")
		case context.DeadlineExceeded:
			return errwrap.Wrapf(err, "ctx deadline exceeded")
		case rpctypes.ErrEmptyKey:
			return errwrap.Wrapf(err, "client-side error")
		default:
			return errwrap.Wrapf(err, "invalid endpoints")
		}
	}
	return nil
}

// txn runs a txn in etcd.
func (obj *Fs) txn(ifcmps []etcd.Cmp, thenops, elseops []etcd.Op) (*etcd.TxnResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), EtcdTimeout)
	resp, err := obj.Client.Txn(ctx).If(ifcmps...).Then(thenops...).Else(elseops...).Commit()
	cancel()
	return resp, err
}

// hash is a small helper that does the hashing for us.
func (obj *Fs) hash(input []byte) (string, error) {
	var h hash.Hash
	switch obj.Hash {
	// TODO: add other hashes
	case "sha256":
		h = sha256.New()
	default:
		return "", fmt.Errorf("hash does not exist")
	}
	src := bytes.NewReader(input)
	if _, err := io.Copy(h, src); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sync overwrites the superblock with whatever version we have stored.
func (obj *Fs) sync() error {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(&obj.sb) // pass with &
	if err != nil {
		return errwrap.Wrapf(err, "gob failed to encode")
	}
	//base64.StdEncoding.EncodeToString(b.Bytes())
	return obj.put(obj.Metadata, b.Bytes())
}

// mount downloads the initial cache of metadata, including the *file tree.
// Since there's no explicit mount API in the afero.Fs interface, we hide this
// method inside any operation that might do any real work, and make it
// idempotent so that it can be called as much as we want. If there's no
// metadata found (superblock) then we create one.
func (obj *Fs) mount() error {
	if obj.mounted {
		return nil
	}

	result, err := obj.get(obj.Metadata) // download the metadata...
	if err != nil {
		return err
	}
	if result == nil || len(result) == 0 { // nothing found, create the fs
		if obj.Debug {
			log.Printf("debug: mount: creating new fs at: %s", obj.Metadata)
		}
		// trim any trailing slashes from DataPrefix
		for strings.HasSuffix(obj.DataPrefix, "/") {
			obj.DataPrefix = strings.TrimSuffix(obj.DataPrefix, "/")
		}
		if obj.DataPrefix == "" {
			obj.DataPrefix = DefaultDataPrefix
		}
		if obj.Hash == "" {
			obj.Hash = DefaultHash
		}
		// test run an empty string to see if our hash selection works!
		if _, err := obj.hash([]byte("")); err != nil {
			return fmt.Errorf("cannot hash with %s", obj.Hash)
		}

		obj.sb = &superBlock{
			DataPrefix: obj.DataPrefix,
			Hash:       obj.Hash,
			Tree: &File{ // include a root directory
				fs:   obj,
				Path: "", // root dir is "" (empty string)
				Mode: os.ModeDir,
			},
		}
		if err := obj.sync(); err != nil {
			return err
		}

		obj.mounted = true
		return nil
	}

	if obj.Debug {
		log.Printf("debug: mount: opening old fs at: %s", obj.Metadata)
	}
	sb, exists := result[obj.Metadata]
	if !exists {
		return fmt.Errorf("could not find metadata") // programming error?
	}

	// decode into obj.sb
	//bb, err := base64.StdEncoding.DecodeString(str)
	//if err != nil {
	//	return errwrap.Wrapf(err, "base64 failed to decode")
	//}
	//b := bytes.NewBuffer(bb)
	b := bytes.NewBuffer(sb)
	d := gob.NewDecoder(b)
	if err := d.Decode(&obj.sb); err != nil { // pass with &
		return errwrap.Wrapf(err, "gob failed to decode")
	}

	if obj.DataPrefix != "" && obj.DataPrefix != obj.sb.DataPrefix {
		return fmt.Errorf("the DataPrefix mount option `%s` does not match the remote value of `%s`", obj.DataPrefix, obj.sb.DataPrefix)
	}
	if obj.Hash != "" && obj.Hash != obj.sb.Hash {
		return fmt.Errorf("the Hash mount option `%s` does not match the remote value of `%s`", obj.Hash, obj.sb.Hash)
	}
	// if all checks passed, copy these values down locally
	obj.DataPrefix = obj.sb.DataPrefix
	obj.Hash = obj.sb.Hash

	// hook up file system pointers to each element in the tree structure
	obj.traverse(obj.sb.Tree)

	obj.mounted = true
	return nil
}

// traverse adds the file system pointer to each element in the tree structure.
func (obj *Fs) traverse(node *File) {
	if node == nil {
		return
	}
	node.fs = obj
	for _, n := range node.Children {
		obj.traverse(n)
	}
}

// find returns the file node corresponding to this absolute path if it exists.
func (obj *Fs) find(absPath string) (*File, error) { // TODO: function naming?
	if absPath == "" {
		return nil, fmt.Errorf("empty path specified")
	}
	if !strings.HasPrefix(absPath, "/") {
		return nil, fmt.Errorf("invalid input path (not absolute)")
	}

	node := obj.sb.Tree
	if node == nil {
		return nil, ErrNotExist // no nodes exist yet, not even root dir
	}

	var x string // first value
	sp := PathSplit(absPath)
	if x, sp = sp[0], sp[1:]; x != node.Path {
		return nil, fmt.Errorf("root values do not match") // TODO: panic?
	}

	for _, p := range sp {
		n, exists := node.findNode(p)
		if !exists {
			return nil, ErrNotExist
		}
		node = n // descend into this node
	}

	return node, nil
}

// Name returns the name of this filesystem.
func (obj *Fs) Name() string { return "etcdfs" }

// URI returns a URI representing this particular filesystem.
func (obj *Fs) URI() string {
	return fmt.Sprintf("%s://%s", obj.Name(), obj.Metadata)
}

// Create creates a new file.
func (obj *Fs) Create(name string) (afero.File, error) {
	if err := obj.mount(); err != nil {
		return nil, err
	}
	return fileCreate(obj, name)
}

// Chown is the equivalent of os.Chown. It returns ErrNotImplemented.
func (obj *Fs) Chown(name string, uid, gid int) error {
	// FIXME: Implement Chown
	return ErrNotImplemented
}

// Lchown is the equivalent of os.Lchown. It returns ErrNotImplemented.
func (obj *Fs) Lchown(name string, uid, gid int) error {
	// FIXME: Implement Lchown
	return ErrNotImplemented
}

// Mkdir makes a new directory.
func (obj *Fs) Mkdir(name string, perm os.FileMode) error {
	if err := obj.mount(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("invalid input path")
	}
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid input path (not absolute)")
	}

	// remove possible trailing slashes
	cleanPath := path.Clean(name)

	for strings.HasSuffix(cleanPath, "/") { // bonus clean for "/" as input
		cleanPath = strings.TrimSuffix(cleanPath, "/")
	}

	if cleanPath == "" {
		if obj.sb.Tree == nil {
			return fmt.Errorf("woops, missing root directory")
		}
		return ErrExist // root directory already exists
	}

	// try to add node to tree by first finding the parent node
	parentPath, dirPath := path.Split(cleanPath) // looking for this

	f := &File{
		fs:   obj,
		Path: dirPath,
		Mode: os.ModeDir,
		// TODO: add perm to struct or let chmod below do it
	}

	node, err := obj.find(parentPath)
	if err != nil { // might be ErrNotExist
		return err
	}

	fi, err := node.Stat()
	if err != nil {
		return err
	}
	if !fi.IsDir() { // is the parent a suitable home?
		return &os.PathError{Op: "mkdir", Path: name, Err: syscall.ENOTDIR}
	}

	_, exists := node.findNode(dirPath) // does file already exist inside?
	if exists {
		return ErrExist
	}

	// add to parent
	node.Children = append(node.Children, f)

	// push new file up if not on server, and then push up the metadata
	if err := f.Sync(); err != nil {
		return err
	}

	return obj.Chmod(name, perm)
}

// MkdirAll creates a directory named path, along with any necessary parents,
// and returns nil, or else returns an error. The permission bits perm are used
// for all directories that MkdirAll creates. If path is already a directory,
// MkdirAll does nothing and returns nil.
func (obj *Fs) MkdirAll(path string, perm os.FileMode) error {
	if err := obj.mount(); err != nil {
		return err
	}
	// Copied mostly verbatim from golang stdlib.
	// Fast path: if we can tell whether path is a directory or file, stop
	// with success or error.
	dir, err := obj.Stat(path)
	if err == nil {
		if dir.IsDir() {
			return nil
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOTDIR}
	}

	// Slow path: make sure parent exists and then call Mkdir for path.
	i := len(path)
	for i > 0 && IsPathSeparator(path[i-1]) { // Skip trailing path separator.
		i--
	}

	j := i
	for j > 0 && !IsPathSeparator(path[j-1]) { // Scan backward over element.
		j--
	}

	if j > 1 {
		// Create parent
		err = obj.MkdirAll(path[0:j-1], perm)
		if err != nil {
			return err
		}
	}

	// Parent now exists; invoke Mkdir and use its result.
	err = obj.Mkdir(path, perm)
	if err != nil {
		// Handle arguments like "foo/." by
		// double-checking that directory doesn't exist.
		dir, err1 := obj.Lstat(path)
		if err1 == nil && dir.IsDir() {
			return nil
		}
		return err
	}
	return nil
}

// Open opens a path. It will be opened read-only.
func (obj *Fs) Open(name string) (afero.File, error) {
	if err := obj.mount(); err != nil {
		return nil, err
	}
	return fileOpen(obj, name) // this opens as read-only
}

// OpenFile opens a path with a particular flag and permission.
func (obj *Fs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if err := obj.mount(); err != nil {
		return nil, err
	}

	chmod := false
	f, err := fileOpen(obj, name)
	if os.IsNotExist(err) && (flag&os.O_CREATE > 0) {
		f, err = fileCreate(obj, name)
		chmod = true
	}
	if err != nil {
		return nil, err
	}
	f.readOnly = (flag == os.O_RDONLY)

	if flag&os.O_APPEND > 0 {
		if _, err := f.Seek(0, os.SEEK_END); err != nil {
			f.Close()
			return nil, err
		}
	}
	if flag&os.O_TRUNC > 0 && flag&(os.O_RDWR|os.O_WRONLY) > 0 {
		if err := f.Truncate(0); err != nil {
			f.Close()
			return nil, err
		}
	}
	if chmod {
		// TODO: the golang stdlib doesn't check this error, should we?
		if err := obj.Chmod(name, perm); err != nil {
			return f, err // TODO: should we return the file handle?
		}
	}
	return f, nil
}

// Remove removes a path.
func (obj *Fs) Remove(name string) error {
	if err := obj.mount(); err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("invalid input path")
	}
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid input path (not absolute)")
	}

	// remove possible trailing slashes
	cleanPath := path.Clean(name)

	for strings.HasSuffix(cleanPath, "/") { // bonus clean for "/" as input
		cleanPath = strings.TrimSuffix(cleanPath, "/")
	}

	if cleanPath == "" {
		return fmt.Errorf("can't remove root")
	}

	f, err := obj.find(name) // get the file
	if err != nil {
		return err
	}

	if len(f.Children) > 0 { // this file or dir has children, can't remove!
		return &os.PathError{Op: "remove", Path: name, Err: syscall.ENOTEMPTY}
	}

	// find the parent node
	parentPath, filePath := path.Split(cleanPath) // looking for this

	node, err := obj.find(parentPath)
	if err != nil { // might be ErrNotExist
		if os.IsNotExist(err) { // race! must have just disappeared
			return nil
		}
		return err
	}

	var index = -1 // int
	for i, n := range node.Children {
		if n.Path == filePath {
			index = i // found here!
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("programming error")
	}
	// remove from list
	node.Children = append(node.Children[:index], node.Children[index+1:]...)
	return obj.sync()
}

// RemoveAll removes path and any children it contains. It removes everything it
// can but returns the first error it encounters. If the path does not exist,
// RemoveAll returns nil (no error).
func (obj *Fs) RemoveAll(path string) error {
	if err := obj.mount(); err != nil {
		return err
	}

	// Simple case: if Remove works, we're done.
	err := obj.Remove(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	// Otherwise, is this a directory we need to recurse into?
	dir, serr := obj.Lstat(path)
	if serr != nil {
		// TODO: I didn't check this logic thoroughly (edge cases?)
		if serr, ok := serr.(*os.PathError); ok && (os.IsNotExist(serr.Err) || serr.Err == syscall.ENOTDIR) {
			return nil
		}
		return serr
	}
	if !dir.IsDir() {
		// Not a directory; return the error from Remove.
		return err
	}

	// Directory.
	fd, err := obj.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Race. It was deleted between the Lstat and Open.
			// Return nil per RemoveAll's docs.
			return nil
		}
		return err
	}

	// Remove contents & return first error.
	err = nil
	for {
		// TODO: why not do this in one shot? is there a syscall limit?
		names, err1 := fd.Readdirnames(100)
		for _, name := range names {
			err1 := obj.RemoveAll(path + string(PathSeparator) + name)
			if err == nil {
				err = err1
			}
		}
		if err1 == io.EOF {
			break
		}
		// If Readdirnames returned an error, use it.
		if err == nil {
			err = err1
		}
		if len(names) == 0 {
			break
		}
	}

	// Close directory, because windows won't remove opened directory.
	fd.Close()

	// Remove directory.
	err1 := obj.Remove(path)
	if err1 == nil || os.IsNotExist(err1) {
		return nil
	}
	if err == nil {
		err = err1
	}
	return err
}

// Rename moves or renames a file or directory.
// TODO: seems it's okay to move files or directories, but you can't clobber dirs
// but you can clobber single files. a dir can't clobber a file and a file can't
// clobber a dir. but a file can clobber another file but a dir can't clobber
// another dir. you can also transplant dirs or files into other dirs.
func (obj *Fs) Rename(oldname, newname string) error {
	// XXX: do we need to check if dest path is inside src path?
	// XXX: if dirs/files are next to each other, do we mess up the .Children list of the common parent?
	if err := obj.mount(); err != nil {
		return err
	}

	if oldname == newname {
		return nil
	}
	if oldname == "" || newname == "" {
		return fmt.Errorf("invalid input path")
	}
	if !strings.HasPrefix(oldname, "/") || !strings.HasPrefix(newname, "/") {
		return fmt.Errorf("invalid input path (not absolute)")
	}

	// remove possible trailing slashes
	srcCleanPath := path.Clean(oldname)
	dstCleanPath := path.Clean(newname)

	src, err := obj.find(srcCleanPath) // get the file
	if err != nil {
		return err
	}

	srcInfo, err := src.Stat()
	if err != nil {
		return err
	}

	srcParentPath, srcName := path.Split(srcCleanPath) // looking for this
	parent, err := obj.find(srcParentPath)
	if err != nil { // might be ErrNotExist
		return err
	}
	var rmi = -1 // index of node to remove from parent
	// find the thing to be deleted
	for i, n := range parent.Children {
		if n.Path == srcName {
			rmi = i // found here!
			break
		}
	}
	if rmi == -1 {
		return fmt.Errorf("programming error")
	}

	dst, err := obj.find(dstCleanPath) // does the destination already exist?
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil { // dst exists!
		dstInfo, err := dst.Stat()
		if err != nil {
			return err
		}

		// dir's can clobber anything or be clobbered apparently
		if srcInfo.IsDir() || dstInfo.IsDir() {
			return ErrExist // dir's can't clobber anything
		}

		// remove from list by index
		parent.Children = append(parent.Children[:rmi], parent.Children[rmi+1:]...)

		// we're a file clobbering another file...
		// move file content from src -> dst and then delete src
		// TODO: run a dst.Close() for extra safety first?
		save := dst.Path // save the "name"
		*dst = *src      // TODO: is this safe?
		dst.Path = save  // "rename" it

	} else { // dst does not exist

		// check if the dst's parent exists and is a dir, if not, error
		// if it is a dir, add src as a child to it and then delete src
		dstParentPath, dstName := path.Split(dstCleanPath) // looking for this
		node, err := obj.find(dstParentPath)
		if err != nil { // might be ErrNotExist
			return err
		}
		fi, err := node.Stat()
		if err != nil {
			return err
		}
		if !fi.IsDir() { // is the parent a suitable home?
			return &os.LinkError{Op: "rename", Old: oldname, New: newname, Err: syscall.ENOTDIR}
		}

		// remove from list by index
		parent.Children = append(parent.Children[:rmi], parent.Children[rmi+1:]...)

		src.Path = dstName                         // "rename" it
		node.Children = append(node.Children, src) // "copied"
	}

	return obj.sync() // push up metadata changes
}

// Stat returns some information about the particular path.
func (obj *Fs) Stat(name string) (os.FileInfo, error) {
	if err := obj.mount(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(name, "/") {
		return nil, fmt.Errorf("invalid input path (not absolute)")
	}

	f, err := obj.find(name) // get the file
	if err != nil {
		return nil, err
	}
	return f.Stat()
}

// Lstat does exactly the same as Stat because we currently do not support
// symbolic links.
func (obj *Fs) Lstat(name string) (os.FileInfo, error) {
	if err := obj.mount(); err != nil {
		return nil, err
	}
	// TODO: we don't have symbolic links in our fs, so we pass this to stat
	return obj.Stat(name)
}

// Chmod changes the mode of a file.
func (obj *Fs) Chmod(name string, mode os.FileMode) error {
	if err := obj.mount(); err != nil {
		return err
	}
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid input path (not absolute)")
	}

	f, err := obj.find(name) // get the file
	if err != nil {
		return err
	}

	f.Mode = f.Mode | mode // XXX: what is the correct way to do this?
	return f.Sync()        // push up the changed metadata
}

// Chtimes changes the access and modification times of the named file, similar
// to the Unix utime() or utimes() functions. The underlying filesystem may
// truncate or round the values to a less precise time unit. If there is an
// error, it will be of type *PathError.
// FIXME: make sure everything we error is a *PathError
// TODO: atime is not currently implement and so it is silently ignored.
func (obj *Fs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if err := obj.mount(); err != nil {
		return err
	}
	if !strings.HasPrefix(name, "/") {
		return fmt.Errorf("invalid input path (not absolute)")
	}

	f, err := obj.find(name) // get the file
	if err != nil {
		return err
	}

	f.ModTime = mtime
	// TODO: add atime
	return f.Sync() // push up the changed metadata
}

// PathSplit splits a path into an array of tokens excluding any trailing empty
// tokens.
func PathSplit(p string) []string {
	if p == "/" { // TODO: can't this all be expressed nicely in one line?
		return []string{""}
	}
	return strings.Split(path.Clean(p), "/")
}

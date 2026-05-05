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

package fs

import (
	"context"
	"io"
	"os"
	"testing"

	"github.com/purpleidea/mgmt/etcd/interfaces"

	"github.com/spf13/afero"
	etcd "go.etcd.io/etcd/client/v3"
)

type countingClient struct {
	gets int
	sets int
	txns int
}

func (obj *countingClient) GetClient() *etcd.Client { return nil }

func (obj *countingClient) GetNamespace() string { return "" }

func (obj *countingClient) Set(ctx context.Context, key, value string, opts ...etcd.OpOption) error {
	obj.sets++
	return nil
}

func (obj *countingClient) Get(ctx context.Context, path string, opts ...etcd.OpOption) (map[string]string, error) {
	obj.gets++
	return map[string]string{}, nil
}

func (obj *countingClient) Del(ctx context.Context, path string, opts ...etcd.OpOption) (int64, error) {
	return 0, nil
}

func (obj *countingClient) Txn(ctx context.Context, ifCmps []etcd.Cmp, thenOps, elseOps []etcd.Op) (*etcd.TxnResponse, error) {
	obj.txns++
	return &etcd.TxnResponse{Succeeded: true}, nil
}

func (obj *countingClient) Watcher(ctx context.Context, path string, opts ...etcd.OpOption) (chan error, error) {
	return nil, nil
}

func (obj *countingClient) ComplexWatcher(ctx context.Context, path string, opts ...etcd.OpOption) (*interfaces.WatcherInfo, error) {
	return nil, nil
}

func (obj *countingClient) WatchMembers(context.Context) (<-chan *interfaces.MembersResult, error) {
	return nil, nil
}

func TestDeferredMetadataFlush(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:        client,
		Metadata:      "/metadata",
		DataPrefix:    DefaultDataPrefix,
		DeferMetadata: true,
	}

	if err := fs.Mkdir("/tmp", 0700); err != nil {
		t.Fatalf("mkdir failed: %+v", err)
	}
	if client.sets != 0 {
		t.Fatalf("expected deferred metadata writes, got %d", client.sets)
	}
	if client.txns != 0 {
		t.Fatalf("expected directory creation to avoid data txns, got %d", client.txns)
	}

	if err := fs.Flush(); err != nil {
		t.Fatalf("flush failed: %+v", err)
	}
	if client.sets != 1 {
		t.Fatalf("expected one metadata write after flush, got %d", client.sets)
	}

	if err := fs.Flush(); err != nil {
		t.Fatalf("second flush failed: %+v", err)
	}
	if client.sets != 1 {
		t.Fatalf("expected clean flush to avoid writes, got %d", client.sets)
	}
}

func TestDeferredMetadataWriteFile(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:        client,
		Metadata:      "/metadata",
		DataPrefix:    DefaultDataPrefix,
		DeferMetadata: true,
	}

	if err := afero.WriteFile(fs, "/file", []byte("hello"), 0600); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if client.sets != 0 {
		t.Fatalf("expected deferred metadata writes, got %d", client.sets)
	}
	if client.txns != 2 {
		t.Fatalf("expected empty and final content data txns only, got %d", client.txns)
	}

	if err := fs.Flush(); err != nil {
		t.Fatalf("flush failed: %+v", err)
	}
	if client.sets != 1 {
		t.Fatalf("expected one metadata write after flush, got %d", client.sets)
	}
}

func TestWritePastEndPreservesExistingData(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("initial write failed: %+v", err)
	}
	if _, err := f.Seek(5, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	if _, err := f.Write([]byte("z")); err != nil {
		t.Fatalf("sparse write failed: %+v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind failed: %+v", err)
	}
	got, err := afero.ReadAll(f)
	if err != nil {
		t.Fatalf("read failed: %+v", err)
	}
	want := []byte{'a', 'b', 'c', 0, 0, 'z'}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteLeavesCursorAfterWrittenBytes(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abcdef")); err != nil {
		t.Fatalf("initial write failed: %+v", err)
	}
	if _, err := f.Seek(2, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	if _, err := f.Write([]byte("XY")); err != nil {
		t.Fatalf("middle write failed: %+v", err)
	}
	if _, err := f.Write([]byte("Z")); err != nil {
		t.Fatalf("follow-up write failed: %+v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind failed: %+v", err)
	}
	got, err := afero.ReadAll(f)
	if err != nil {
		t.Fatalf("read failed: %+v", err)
	}
	if want := "abXYZf"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadPastEndReturnsEOF(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if _, err := f.Seek(10, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	buf := make([]byte, 1)
	n, err := f.Read(buf)
	if n != 0 || err != io.EOF {
		t.Fatalf("read got n=%d err=%v, want n=0 err=%v", n, err, io.EOF)
	}
}

func TestSeekRejectsNegativeOffset(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if _, err := f.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("initial seek failed: %+v", err)
	}
	if off, err := f.Seek(-2, io.SeekCurrent); err == nil {
		t.Fatalf("negative seek got offset=%d err=nil, want error", off)
	}
	off, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatalf("current seek failed: %+v", err)
	}
	if off != 1 {
		t.Fatalf("offset changed after failed seek: got %d, want 1", off)
	}
}

func TestChmodCanRemovePermissionBits(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	if _, err := fs.Create("/file"); err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if err := fs.Chmod("/file", 0777); err != nil {
		t.Fatalf("wide chmod failed: %+v", err)
	}
	if err := fs.Chmod("/file", 0600); err != nil {
		t.Fatalf("tight chmod failed: %+v", err)
	}
	fi, err := fs.Stat("/file")
	if err != nil {
		t.Fatalf("stat failed: %+v", err)
	}
	if got, want := fi.Mode().Perm(), os.FileMode(0600); got != want {
		t.Fatalf("mode got %v, want %v", got, want)
	}
}

func TestOpenFileCreateExclFailsWhenFileExists(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	if err := afero.WriteFile(fs, "/file", []byte("contents"), 0600); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	f, err := fs.OpenFile("/file", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		f.Close()
		t.Fatalf("openfile with O_CREATE|O_EXCL succeeded, want error")
	}
	if !os.IsExist(err) {
		t.Fatalf("openfile got err=%v, want exists error", err)
	}
}

func TestOpenFileAppendWritesAtEndAfterSeek(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	if err := afero.WriteFile(fs, "/file", []byte("abc"), 0600); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	f, err := fs.OpenFile("/file", os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("openfile failed: %+v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	if _, err := f.Write([]byte("z")); err != nil {
		t.Fatalf("append write failed: %+v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind failed: %+v", err)
	}
	got, err := afero.ReadAll(f)
	if err != nil {
		t.Fatalf("read failed: %+v", err)
	}
	if want := "abcz"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReadAtDoesNotChangeCursor(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abcdef")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if _, err := f.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	buf := make([]byte, 2)
	if n, err := f.ReadAt(buf, 4); n != 2 || err != nil {
		t.Fatalf("readat got n=%d err=%v, want n=2 err=nil", n, err)
	}
	if string(buf) != "ef" {
		t.Fatalf("readat got %q, want %q", buf, "ef")
	}
	buf = make([]byte, 2)
	if n, err := f.Read(buf); n != 2 || err != nil {
		t.Fatalf("read got n=%d err=%v, want n=2 err=nil", n, err)
	}
	if string(buf) != "bc" {
		t.Fatalf("read got %q, want %q", buf, "bc")
	}
}

func TestReadAtShortReadReturnsEOF(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	buf := make([]byte, 4)
	n, err := f.ReadAt(buf, 1)
	if n != 2 || err != io.EOF {
		t.Fatalf("readat got n=%d err=%v, want n=2 err=%v", n, err, io.EOF)
	}
	if string(buf[:n]) != "bc" {
		t.Fatalf("readat got %q, want %q", buf[:n], "bc")
	}
}

func TestReadAtRejectsNegativeOffset(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	buf := make([]byte, 1)
	if n, err := f.ReadAt(buf, -1); n != 0 || err == nil {
		t.Fatalf("readat got n=%d err=%v, want n=0 and an error", n, err)
	}
}

func TestWriteAtDoesNotChangeCursor(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abcdef")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if _, err := f.Seek(1, io.SeekStart); err != nil {
		t.Fatalf("seek failed: %+v", err)
	}
	if n, err := f.WriteAt([]byte("XY"), 4); n != 2 || err != nil {
		t.Fatalf("writeat got n=%d err=%v, want n=2 err=nil", n, err)
	}
	if _, err := f.Write([]byte("z")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind failed: %+v", err)
	}
	got, err := afero.ReadAll(f)
	if err != nil {
		t.Fatalf("read failed: %+v", err)
	}
	if want := "azcdXY"; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteAtRejectsNegativeOffset(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.Create("/file")
	if err != nil {
		t.Fatalf("create failed: %+v", err)
	}
	if _, err := f.Write([]byte("abc")); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	if n, err := f.WriteAt([]byte("z"), -1); n != 0 || err == nil {
		t.Fatalf("writeat got n=%d err=%v, want n=0 and an error", n, err)
	}
}

func TestOpenFileWriteOnlyRejectsRead(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	if err := afero.WriteFile(fs, "/file", []byte("abc"), 0600); err != nil {
		t.Fatalf("write failed: %+v", err)
	}
	f, err := fs.OpenFile("/file", os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("openfile failed: %+v", err)
	}
	buf := make([]byte, 1)
	if n, err := f.Read(buf); n != 0 || err == nil {
		t.Fatalf("read got n=%d err=%v, want n=0 and an error", n, err)
	}
}

func TestOpenFileReadOnlyCreateRejectsWrite(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	f, err := fs.OpenFile("/file", os.O_RDONLY|os.O_CREATE, 0600)
	if err != nil {
		t.Fatalf("openfile failed: %+v", err)
	}
	if n, err := f.Write([]byte("z")); n != 0 || err == nil {
		t.Fatalf("write got n=%d err=%v, want n=0 and an error", n, err)
	}
}

func TestRenameDirectoryIntoDescendantFails(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:        client,
		Metadata:      "/metadata",
		DataPrefix:    DefaultDataPrefix,
		DeferMetadata: true,
	}

	if err := fs.MkdirAll("/a/b", 0700); err != nil {
		t.Fatalf("mkdirall failed: %+v", err)
	}
	if err := fs.Rename("/a", "/a/b/c"); err == nil {
		t.Fatalf("rename directory into descendant succeeded, want error")
	}
}

func TestRenameRootFails(t *testing.T) {
	client := &countingClient{}
	fs := &Fs{
		Client:     client,
		Metadata:   "/metadata",
		DataPrefix: DefaultDataPrefix,
	}

	err := fs.Rename("/", "/root")
	if err == nil {
		t.Fatalf("rename root succeeded, want error")
	}
	if _, ok := err.(*os.LinkError); !ok {
		t.Fatalf("rename root got %T %v, want *os.LinkError", err, err)
	}
}

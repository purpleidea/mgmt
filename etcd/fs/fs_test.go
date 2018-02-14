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

package fs_test // named this way to make it easier for examples

import (
	"io"
	"testing"

	"github.com/purpleidea/mgmt/etcd"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/util"

	"github.com/spf13/afero"
)

// XXX: spawn etcd for this test, like `cdtmpmkdir && etcd` and then kill it...
// XXX: write a bunch more tests to test this

// TODO: apparently using 0666 is equivalent to respecting the current umask
const (
	umask      = 0666
	superblock = "/some/superblock" // TODO: generate randomly per test?
)

func TestFs1(t *testing.T) {
	etcdClient := &etcd.ClientEtcd{
		Seeds: []string{"localhost:2379"}, // endpoints
	}

	if err := etcdClient.Connect(); err != nil {
		t.Logf("client connection error: %+v", err)
		return
	}
	defer etcdClient.Destroy()

	etcdFs := &etcdfs.Fs{
		Client:     etcdClient.GetClient(),
		Metadata:   superblock,
		DataPrefix: etcdfs.DefaultDataPrefix,
	}
	//var etcdFs afero.Fs = NewEtcdFs()

	if err := etcdFs.Mkdir("/", umask); err != nil {
		t.Logf("error: %+v", err)
		if err != etcdfs.ErrExist {
			return
		}
	}

	if err := etcdFs.Mkdir("/tmp", umask); err != nil {
		t.Logf("error: %+v", err)
		if err != etcdfs.ErrExist {
			return
		}
	}

	fi, err := etcdFs.Stat("/tmp")
	if err != nil {
		t.Logf("stat error: %+v", err)
		return
	}

	t.Logf("fi: %+v", fi)
	t.Logf("isdir: %t", fi.IsDir())

	f, err := etcdFs.Create("/tmp/foo")
	if err != nil {
		t.Logf("error: %+v", err)
		return
	}

	t.Logf("handle: %+v", f)

	i, err := f.WriteString("hello world!\n")
	if err != nil {
		t.Logf("error: %+v", err)
		return
	}
	t.Logf("wrote: %d", i)

	if err := etcdFs.Mkdir("/tmp/d1", umask); err != nil {
		t.Logf("error: %+v", err)
		if err != etcdfs.ErrExist {
			return
		}
	}

	if err := etcdFs.Rename("/tmp/foo", "/tmp/bar"); err != nil {
		t.Logf("rename error: %+v", err)
		return
	}

	//f2, err := etcdFs.Create("/tmp/bar")
	//if err != nil {
	//	t.Logf("error: %+v", err)
	//	return
	//}

	//i2, err := f2.WriteString("hello bar!\n")
	//if err != nil {
	//	t.Logf("error: %+v", err)
	//	return
	//}
	//t.Logf("wrote: %d", i2)

	dir, err := etcdFs.Open("/tmp")
	if err != nil {
		t.Logf("error: %+v", err)
		return
	}
	names, err := dir.Readdirnames(-1)
	if err != nil && err != io.EOF {
		t.Logf("error: %+v", err)
		return
	}
	for _, name := range names {
		t.Logf("name in /tmp: %+v", name)
	}

	//dir, err := etcdFs.Open("/")
	//if err != nil {
	//	t.Logf("error: %+v", err)
	//	return
	//}
	//names, err := dir.Readdirnames(-1)
	//if err != nil && err != io.EOF {
	//	t.Logf("error: %+v", err)
	//	return
	//}
	//for _, name := range names {
	//	t.Logf("name in /: %+v", name)
	//}
}

func TestFs2(t *testing.T) {
	etcdClient := &etcd.ClientEtcd{
		Seeds: []string{"localhost:2379"}, // endpoints
	}

	if err := etcdClient.Connect(); err != nil {
		t.Logf("client connection error: %+v", err)
		return
	}
	defer etcdClient.Destroy()

	etcdFs := &etcdfs.Fs{
		Client:     etcdClient.GetClient(),
		Metadata:   superblock,
		DataPrefix: etcdfs.DefaultDataPrefix,
	}

	tree, err := util.FsTree(etcdFs, "/")
	if err != nil {
		t.Errorf("tree error: %+v", err)
		return
	}
	t.Logf("tree: \n%s", tree)

	tree2, err := util.FsTree(etcdFs, "/tmp")
	if err != nil {
		t.Errorf("tree2 error: %+v", err)
		return
	}
	t.Logf("tree2: \n%s", tree2)
}

func TestFs3(t *testing.T) {
	etcdClient := &etcd.ClientEtcd{
		Seeds: []string{"localhost:2379"}, // endpoints
	}

	if err := etcdClient.Connect(); err != nil {
		t.Logf("client connection error: %+v", err)
		return
	}
	defer etcdClient.Destroy()

	etcdFs := &etcdfs.Fs{
		Client:     etcdClient.GetClient(),
		Metadata:   superblock,
		DataPrefix: etcdfs.DefaultDataPrefix,
	}

	tree, err := util.FsTree(etcdFs, "/")
	if err != nil {
		t.Errorf("tree error: %+v", err)
		return
	}
	t.Logf("tree: \n%s", tree)

	var memFs = afero.NewMemMapFs()

	if err := util.CopyFs(etcdFs, memFs, "/", "/", false); err != nil {
		t.Errorf("CopyFs error: %+v", err)
		return
	}
	if err := util.CopyFs(etcdFs, memFs, "/", "/", true); err != nil {
		t.Errorf("CopyFs2 error: %+v", err)
		return
	}
	if err := util.CopyFs(etcdFs, memFs, "/", "/tmp/d1/", false); err != nil {
		t.Errorf("CopyFs3 error: %+v", err)
		return
	}

	tree2, err := util.FsTree(memFs, "/")
	if err != nil {
		t.Errorf("tree2 error: %+v", err)
		return
	}
	t.Logf("tree2: \n%s", tree2)
}

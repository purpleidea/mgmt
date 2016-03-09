// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

func TestMiscT1(t *testing.T) {

	if Dirname("/foo/bar/baz") != "/foo/bar/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/foo/bar/baz/") != "/foo/bar/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/foo/") != "/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/") != "" { // TODO: should this equal "/" or "" ?
		t.Errorf("Result is incorrect.")
	}

	if Basename("/foo/bar/baz") != "baz" {
		t.Errorf("Result is incorrect.")
	}

	if Basename("/foo/bar/baz/") != "baz/" {
		t.Errorf("Result is incorrect.")
	}

	if Basename("/foo/") != "foo/" {
		t.Errorf("Result is incorrect.")
	}

	if Basename("/") != "/" { // TODO: should this equal "" or "/" ?
		t.Errorf("Result is incorrect.")
	}

}

func TestMiscT2(t *testing.T) {

	// TODO: compare the output with the actual list
	p0 := "/"
	r0 := []string{""} // TODO: is this correct?
	if len(PathSplit(p0)) != len(r0) {
		t.Errorf("Result should be: %q.", r0)
		t.Errorf("Result should have a length of: %v.", len(r0))
	}

	p1 := "/foo/bar/baz"
	r1 := []string{"", "foo", "bar", "baz"}
	if len(PathSplit(p1)) != len(r1) {
		//t.Errorf("Result should be: %q.", r1)
		t.Errorf("Result should have a length of: %v.", len(r1))
	}

	p2 := "/foo/bar/baz/"
	r2 := []string{"", "foo", "bar", "baz"}
	if len(PathSplit(p2)) != len(r2) {
		t.Errorf("Result should have a length of: %v.", len(r2))
	}
}

func TestMiscT3(t *testing.T) {

	if HasPathPrefix("/foo/bar/baz", "/foo/ba") != false {
		t.Errorf("Result should be false.")
	}

	if HasPathPrefix("/foo/bar/baz", "/foo/bar") != true {
		t.Errorf("Result should be true.")
	}

	if HasPathPrefix("/foo/bar/baz", "/foo/bar/") != true {
		t.Errorf("Result should be true.")
	}

	if HasPathPrefix("/foo/bar/baz/", "/foo/bar") != true {
		t.Errorf("Result should be true.")
	}

	if HasPathPrefix("/foo/bar/baz/", "/foo/bar/") != true {
		t.Errorf("Result should be true.")
	}

	if HasPathPrefix("/foo/bar/baz/", "/foo/bar/baz/dude") != false {
		t.Errorf("Result should be false.")
	}

	if HasPathPrefix("/foo/bar/baz/boo/", "/foo/") != true {
		t.Errorf("Result should be true.")
	}
}

func TestMiscT4(t *testing.T) {

	if PathPrefixDelta("/foo/bar/baz", "/foo/ba") != -1 {
		t.Errorf("Result should be -1.")
	}

	if PathPrefixDelta("/foo/bar/baz", "/foo/bar") != 1 {
		t.Errorf("Result should be 1.")
	}

	if PathPrefixDelta("/foo/bar/baz", "/foo/bar/") != 1 {
		t.Errorf("Result should be 1.")
	}

	if PathPrefixDelta("/foo/bar/baz/", "/foo/bar") != 1 {
		t.Errorf("Result should be 1.")
	}

	if PathPrefixDelta("/foo/bar/baz/", "/foo/bar/") != 1 {
		t.Errorf("Result should be 1.")
	}

	if PathPrefixDelta("/foo/bar/baz/", "/foo/bar/baz/dude") != -1 {
		t.Errorf("Result should be -1.")
	}

	if PathPrefixDelta("/foo/bar/baz/a/b/c/", "/foo/bar/baz") != 3 {
		t.Errorf("Result should be 3.")
	}

	if PathPrefixDelta("/foo/bar/baz/", "/foo/bar/baz") != 0 {
		t.Errorf("Result should be 0.")
	}
}

func TestMiscT5(t *testing.T) {

	if PathIsDir("/foo/bar/baz/") != true {
		t.Errorf("Result should be false.")
	}

	if PathIsDir("/foo/bar/baz") != false {
		t.Errorf("Result should be false.")
	}

	if PathIsDir("/foo/") != true {
		t.Errorf("Result should be true.")
	}

	if PathIsDir("/") != true {
		t.Errorf("Result should be true.")
	}
}

func TestMiscT6(t *testing.T) {

	type foo struct {
		Name  string `yaml:"name"`
		Res   string `yaml:"res"`
		Value int    `yaml:"value"`
	}

	obj := foo{"dude", "sweet", 42}
	output, ok := ObjToB64(obj)
	if ok != true {
		t.Errorf("First result should be true.")
	}
	var data foo
	if B64ToObj(output, &data) != true {
		t.Errorf("Second result should be true.")
	}
	// TODO: there is probably a better way to compare these two...
	if fmt.Sprintf("%+v\n", obj) != fmt.Sprintf("%+v\n", data) {
		t.Errorf("Strings should match.")
	}
}

func TestMiscT7(t *testing.T) {

	type Foo struct {
		Name  string `yaml:"name"`
		Res   string `yaml:"res"`
		Value int    `yaml:"value"`
	}

	type bar struct {
		Foo     `yaml:",inline"` // anonymous struct must be public!
		Comment string           `yaml:"comment"`
	}

	obj := bar{Foo{"dude", "sweet", 42}, "hello world"}
	output, ok := ObjToB64(obj)
	if ok != true {
		t.Errorf("First result should be true.")
	}
	var data bar
	if B64ToObj(output, &data) != true {
		t.Errorf("Second result should be true.")
	}
	// TODO: there is probably a better way to compare these two...
	if fmt.Sprintf("%+v\n", obj) != fmt.Sprintf("%+v\n", data) {
		t.Errorf("Strings should match.")
	}
}

func TestMiscT8(t *testing.T) {

	r0 := []string{"/"}
	if fullList0 := PathSplitFullReversed("/"); !reflect.DeepEqual(r0, fullList0) {
		t.Errorf("PathSplitFullReversed expected: %v; got: %v.", r0, fullList0)
	}

	r1 := []string{"/foo/bar/baz/file", "/foo/bar/baz/", "/foo/bar/", "/foo/", "/"}
	if fullList1 := PathSplitFullReversed("/foo/bar/baz/file"); !reflect.DeepEqual(r1, fullList1) {
		t.Errorf("PathSplitFullReversed expected: %v; got: %v.", r1, fullList1)
	}

	r2 := []string{"/foo/bar/baz/dir/", "/foo/bar/baz/", "/foo/bar/", "/foo/", "/"}
	if fullList2 := PathSplitFullReversed("/foo/bar/baz/dir/"); !reflect.DeepEqual(r2, fullList2) {
		t.Errorf("PathSplitFullReversed expected: %v; got: %v.", r2, fullList2)
	}

}

func TestMiscT9(t *testing.T) {
	fileListIn := []string{ // list taken from drbd-utils package
		"/etc/drbd.conf",
		"/etc/drbd.d/global_common.conf",
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/etc/drbd.d",
		"/usr/share/doc/drbd-utils",
		"/var/lib/drbd",
	}
	sort.Strings(fileListIn)

	fileListOut := []string{ // fixed up manually
		"/etc/drbd.conf",
		"/etc/drbd.d/global_common.conf",
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/etc/drbd.d/",               // added trailing slash
		"/usr/share/doc/drbd-utils/", // added trailing slash
		"/var/lib/drbd",              // can't be fixed :(
	}
	sort.Strings(fileListOut)

	dirify := DirifyFileList(fileListIn, false) // TODO: test with true
	sort.Strings(dirify)
	equals := reflect.DeepEqual(fileListOut, dirify)
	if a, b := len(fileListOut), len(dirify); a != b {
		t.Errorf("DirifyFileList counts didn't match: %d != %d", a, b)
	} else if !equals {
		t.Error("DirifyFileList did not match expected!")
		for i := 0; i < len(dirify); i++ {
			if fileListOut[i] != dirify[i] {
				t.Errorf("# %d: %v <> %v", i, fileListOut[i], dirify[i])
			}
		}
	}
}

func TestMiscT10(t *testing.T) {
	fileListIn := []string{ // fake package list
		"/etc/drbd.conf",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/etc/drbd.d",
		"/etc/drbd.d/foo",
		"/var/lib/drbd",
		"/var/somedir/",
	}
	sort.Strings(fileListIn)

	fileListOut := []string{ // fixed up manually
		"/etc/drbd.conf",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/etc/drbd.d/", // added trailing slash
		"/etc/drbd.d/foo",
		"/var/lib/drbd", // can't be fixed :(
		"/var/somedir/", // stays the same
	}
	sort.Strings(fileListOut)

	dirify := DirifyFileList(fileListIn, false) // TODO: test with true
	sort.Strings(dirify)
	equals := reflect.DeepEqual(fileListOut, dirify)
	if a, b := len(fileListOut), len(dirify); a != b {
		t.Errorf("DirifyFileList counts didn't match: %d != %d", a, b)
	} else if !equals {
		t.Error("DirifyFileList did not match expected!")
		for i := 0; i < len(dirify); i++ {
			if fileListOut[i] != dirify[i] {
				t.Errorf("# %d: %v <> %v", i, fileListOut[i], dirify[i])
			}
		}
	}
}

func TestMiscT11(t *testing.T) {
	in1 := []string{"/", "/usr/", "/usr/lib/", "/usr/share/"} // input
	ex1 := []string{"/usr/lib/", "/usr/share/"}               // expected
	sort.Strings(ex1)
	out1 := RemoveCommonFilePrefixes(in1)
	sort.Strings(out1)
	if !reflect.DeepEqual(ex1, out1) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex1, out1)
	}

	in2 := []string{"/", "/usr/"}
	ex2 := []string{"/usr/"}
	sort.Strings(ex2)
	out2 := RemoveCommonFilePrefixes(in2)
	sort.Strings(out2)
	if !reflect.DeepEqual(ex2, out2) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex2, out2)
	}

	in3 := []string{"/"}
	ex3 := []string{"/"}
	out3 := RemoveCommonFilePrefixes(in3)
	if !reflect.DeepEqual(ex3, out3) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex3, out3)
	}

	in4 := []string{"/usr/bin/foo", "/usr/bin/bar", "/usr/lib/", "/usr/share/"}
	ex4 := []string{"/usr/bin/foo", "/usr/bin/bar", "/usr/lib/", "/usr/share/"}
	sort.Strings(ex4)
	out4 := RemoveCommonFilePrefixes(in4)
	sort.Strings(out4)
	if !reflect.DeepEqual(ex4, out4) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex4, out4)
	}

	in5 := []string{"/usr/bin/foo", "/usr/bin/bar", "/usr/lib/", "/usr/share/", "/usr/bin"}
	ex5 := []string{"/usr/bin/foo", "/usr/bin/bar", "/usr/lib/", "/usr/share/"}
	sort.Strings(ex5)
	out5 := RemoveCommonFilePrefixes(in5)
	sort.Strings(out5)
	if !reflect.DeepEqual(ex5, out5) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex5, out5)
	}

	in6 := []string{"/etc/drbd.d/", "/lib/drbd/", "/usr/lib/drbd/", "/usr/lib/systemd/system/", "/usr/lib/tmpfiles.d/", "/usr/sbin/", "/usr/share/doc/drbd-utils/", "/usr/share/man/man5/", "/usr/share/man/man8/", "/usr/share/doc/", "/var/lib/"}
	ex6 := []string{"/etc/drbd.d/", "/lib/drbd/", "/usr/lib/drbd/", "/usr/lib/systemd/system/", "/usr/lib/tmpfiles.d/", "/usr/sbin/", "/usr/share/doc/drbd-utils/", "/usr/share/man/man5/", "/usr/share/man/man8/", "/var/lib/"}
	sort.Strings(ex6)
	out6 := RemoveCommonFilePrefixes(in6)
	sort.Strings(out6)
	if !reflect.DeepEqual(ex6, out6) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex6, out6)
	}

	in7 := []string{"/etc/", "/lib/", "/usr/lib/", "/usr/lib/systemd/", "/usr/", "/usr/share/doc/", "/usr/share/man/", "/var/"}
	ex7 := []string{"/etc/", "/lib/", "/usr/lib/systemd/", "/usr/share/doc/", "/usr/share/man/", "/var/"}
	sort.Strings(ex7)
	out7 := RemoveCommonFilePrefixes(in7)
	sort.Strings(out7)
	if !reflect.DeepEqual(ex7, out7) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex7, out7)
	}

	in8 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/global_common.conf",
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/etc/drbd.d/",
		"/usr/share/doc/drbd-utils/",
		"/var/lib/drbd",
	}
	ex8 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/global_common.conf",
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/var/lib/drbd",
	}
	sort.Strings(ex8)
	out8 := RemoveCommonFilePrefixes(in8)
	sort.Strings(out8)
	if !reflect.DeepEqual(ex8, out8) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex8, out8)
	}

	in9 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/",
		"/lib/drbd/drbd",
		"/lib/drbd/",
		"/lib/drbd/",
		"/lib/drbd/",
		"/usr/lib/drbd/",
		"/usr/lib/drbd/",
		"/usr/lib/drbd/",
		"/usr/lib/drbd/",
		"/usr/lib/drbd/",
		"/usr/lib/systemd/system/",
		"/usr/lib/tmpfiles.d/",
		"/usr/sbin/",
		"/usr/sbin/",
		"/usr/share/doc/drbd-utils/",
		"/usr/share/doc/drbd-utils/",
		"/usr/share/man/man5/",
		"/usr/share/man/man5/",
		"/usr/share/man/man8/",
		"/usr/share/man/man8/",
		"/usr/share/man/man8/",
		"/etc/drbd.d/",
		"/usr/share/doc/drbd-utils/",
		"/var/lib/drbd",
	}
	ex9 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/",
		"/lib/drbd/drbd",
		"/usr/lib/drbd/",
		"/usr/lib/systemd/system/",
		"/usr/lib/tmpfiles.d/",
		"/usr/sbin/",
		"/usr/share/doc/drbd-utils/",
		"/usr/share/man/man5/",
		"/usr/share/man/man8/",
		"/var/lib/drbd",
	}
	sort.Strings(ex9)
	out9 := RemoveCommonFilePrefixes(in9)
	sort.Strings(out9)
	if !reflect.DeepEqual(ex9, out9) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex9, out9)
	}

	in10 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/",                   // watch me, i'm a dir
		"/etc/drbd.d/global_common.conf", // and watch me i'm a file!
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/", // watch me, i'm a dir too
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/var/lib/drbd",
	}
	ex10 := []string{
		"/etc/drbd.conf",
		"/etc/drbd.d/global_common.conf",
		"/lib/drbd/drbd",
		"/lib/drbd/drbdadm-83",
		"/lib/drbd/drbdadm-84",
		"/lib/drbd/drbdsetup-83",
		"/lib/drbd/drbdsetup-84",
		"/usr/lib/drbd/crm-fence-peer.sh",
		"/usr/lib/drbd/crm-unfence-peer.sh",
		"/usr/lib/drbd/notify-emergency-reboot.sh",
		"/usr/lib/drbd/notify-emergency-shutdown.sh",
		"/usr/lib/drbd/notify-io-error.sh",
		"/usr/lib/drbd/notify-out-of-sync.sh",
		"/usr/lib/drbd/notify-pri-lost-after-sb.sh",
		"/usr/lib/drbd/notify-pri-lost.sh",
		"/usr/lib/drbd/notify-pri-on-incon-degr.sh",
		"/usr/lib/drbd/notify-split-brain.sh",
		"/usr/lib/drbd/notify.sh",
		"/usr/lib/drbd/outdate-peer.sh",
		"/usr/lib/drbd/rhcs_fence",
		"/usr/lib/drbd/snapshot-resync-target-lvm.sh",
		"/usr/lib/drbd/stonith_admin-fence-peer.sh",
		"/usr/lib/drbd/unsnapshot-resync-target-lvm.sh",
		"/usr/lib/systemd/system/drbd.service",
		"/usr/lib/tmpfiles.d/drbd.conf",
		"/usr/sbin/drbd-overview",
		"/usr/sbin/drbdadm",
		"/usr/sbin/drbdmeta",
		"/usr/sbin/drbdsetup",
		"/usr/share/doc/drbd-utils/COPYING",
		"/usr/share/doc/drbd-utils/ChangeLog",
		"/usr/share/doc/drbd-utils/README",
		"/usr/share/doc/drbd-utils/drbd.conf.example",
		"/usr/share/man/man5/drbd.conf-8.3.5.gz",
		"/usr/share/man/man5/drbd.conf-8.4.5.gz",
		"/usr/share/man/man5/drbd.conf-9.0.5.gz",
		"/usr/share/man/man5/drbd.conf.5.gz",
		"/usr/share/man/man8/drbd-8.3.8.gz",
		"/usr/share/man/man8/drbd-8.4.8.gz",
		"/usr/share/man/man8/drbd-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview-9.0.8.gz",
		"/usr/share/man/man8/drbd-overview.8.gz",
		"/usr/share/man/man8/drbd.8.gz",
		"/usr/share/man/man8/drbdadm-8.3.8.gz",
		"/usr/share/man/man8/drbdadm-8.4.8.gz",
		"/usr/share/man/man8/drbdadm-9.0.8.gz",
		"/usr/share/man/man8/drbdadm.8.gz",
		"/usr/share/man/man8/drbddisk-8.3.8.gz",
		"/usr/share/man/man8/drbddisk-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-8.3.8.gz",
		"/usr/share/man/man8/drbdmeta-8.4.8.gz",
		"/usr/share/man/man8/drbdmeta-9.0.8.gz",
		"/usr/share/man/man8/drbdmeta.8.gz",
		"/usr/share/man/man8/drbdsetup-8.3.8.gz",
		"/usr/share/man/man8/drbdsetup-8.4.8.gz",
		"/usr/share/man/man8/drbdsetup-9.0.8.gz",
		"/usr/share/man/man8/drbdsetup.8.gz",
		"/var/lib/drbd",
	}
	sort.Strings(ex10)
	out10 := RemoveCommonFilePrefixes(in10)
	sort.Strings(out10)
	if !reflect.DeepEqual(ex10, out10) {
		t.Errorf("RemoveCommonFilePrefixes expected: %v; got: %v.", ex10, out10)
		for i := 0; i < len(ex10); i++ {
			if ex10[i] != out10[i] {
				t.Errorf("# %d: %v <> %v", i, ex10[i], out10[i])
			}
		}
	}
}

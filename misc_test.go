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
	"testing"
)

func TestMiscT1(t *testing.T) {

	if Dirname("/foo/bar/baz") != "/foo/bar/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/foo/bar/baz/") != "/foo/bar/" {
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

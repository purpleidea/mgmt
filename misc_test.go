// Mgmt
// Copyright (C) 2013-2015+ James Shubin and the project contributors
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
	"testing"
)

func TestMiscT1(t *testing.T) {

	if Dirname("/foo/bar/baz") != "/foo/bar/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/foo/bar/baz/") != "/foo/bar/" {
		t.Errorf("Result is incorrect.")
	}

	if Dirname("/") != "/" {
		t.Errorf("Result is incorrect.")
	}
}

func TestMiscT2(t *testing.T) {

	// TODO: compare the output with the actual list
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

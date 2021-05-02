// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package safepath_test

import (
	"testing"

	"github.com/purpleidea/mgmt/util/safepath"
)

type badPath struct{}

func (obj badPath) String() string { return "" }
func (obj badPath) Path() string   { return "" }
func (obj badPath) IsDir() bool    { return false }
func (obj badPath) IsAbs() bool    { return false }
func (obj badPath) isPath()        {} // can't be the same as in the Path interface!

func TestInterfaces(t *testing.T) {

	absFile, err := safepath.ParseIntoAbsFile("/foo/bar/abs/file")
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}

	absDir, err := safepath.ParseIntoAbsDir("/foo/bar/abs/dir/")
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}

	relFile, err := safepath.ParseIntoRelFile("foo/bar/rel/file")
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}

	relDir, err := safepath.ParseIntoRelDir("foo/bar/rel/dir/")
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}

	var p safepath.Path
	p = absFile
	p = absDir
	p = relFile
	p = relDir
	//p = badPath{} // nope
	_ = p

	var f safepath.File
	f = absFile
	//f = absDir // nope
	f = relFile
	//f = relDir // nope
	_ = f

	var d safepath.Dir
	//d = absFile // nope
	d = absDir
	//d = relFile // nope
	d = relDir
	_ = d

	var a safepath.Abs
	a = absFile
	a = absDir
	//a = relFile // nope
	//a = relDir // nope
	_ = a

	var r safepath.Rel
	//r = absFile // nope
	//r = absDir // nope
	r = relFile
	r = relDir
	_ = r
}

func TestParse(t *testing.T) {
	path, err := safepath.ParseIntoPath("/foo/bar/abs/file", false)
	if err != nil {
		t.Errorf("err: %+v", err)
		return
	}

	if _, ok := path.(safepath.AbsFile); !ok {
		t.Errorf("expected AbsFile, got: %T", path)
	}
	if _, ok := path.(safepath.AbsDir); ok {
		t.Errorf("unexpected AbsDir")
	}
	if _, ok := path.(safepath.RelFile); ok {
		t.Errorf("unexpected RelFile")
	}
	if _, ok := path.(safepath.RelDir); ok {
		t.Errorf("unexpected RelDir")
	}
}

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

//go:build !root

package resources

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/pgraph"
)

func TestFileAutoEdge1(t *testing.T) {

	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	r1 := &FileRes{
		Path: "/tmp/a/b/", // some dir
	}
	r2 := &FileRes{
		Path: "/tmp/a/", // some parent dir
	}
	r3 := &FileRes{
		Path: "/tmp/a/b/c", // some child file
	}
	g.AddVertex(r1, r2, r3)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
	}

	debug := testing.Verbose() // set via the -test.v flag to `go test`
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	// run artificially without the entire engine
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
	}

	// two edges should have been added
	if i := g.NumEdges(); i != 2 {
		t.Errorf("should have 2 edges instead of: %d", i)
	}
}

func TestMiscEncodeDecode1(t *testing.T) {
	var err error

	// encode
	var input interface{} = &FileRes{}
	b1 := bytes.Buffer{}
	e := gob.NewEncoder(&b1)
	err = e.Encode(&input) // pass with &
	if err != nil {
		t.Errorf("gob failed to Encode: %v", err)
	}
	str := base64.StdEncoding.EncodeToString(b1.Bytes())

	// decode
	var output interface{}
	bb, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		t.Errorf("base64 failed to Decode: %v", err)
	}
	b2 := bytes.NewBuffer(bb)
	d := gob.NewDecoder(b2)
	err = d.Decode(&output) // pass with &
	if err != nil {
		t.Errorf("gob failed to Decode: %v", err)
	}

	res1, ok := input.(engine.Res)
	if !ok {
		t.Errorf("input %v is not a Res", res1)
		return
	}
	res2, ok := output.(engine.Res)
	if !ok {
		t.Errorf("output %v is not a Res", res2)
		return
	}
	if err := res1.Cmp(res2); err != nil {
		t.Errorf("the input and output Res values do not match: %+v", err)
	}
}

func TestMiscEncodeDecode2(t *testing.T) {
	var err error

	// encode
	input, err := engine.NewNamedResource("file", "file1")
	if err != nil {
		t.Errorf("can't create: %v", err)
		return
	}
	// NOTE: Do not add this bit of code, because it would cause the path to
	// get taken from the actual Path parameter, instead of using the name,
	// and if we use the name, the Cmp function will detect if the name is
	// stored properly or not.
	//fileRes := input.(*FileRes) // must not panic
	//fileRes.Path = "/tmp/whatever"

	b64, err := engineUtil.ResToB64(input)
	if err != nil {
		t.Errorf("can't encode: %v", err)
		return
	}

	output, err := engineUtil.B64ToRes(b64)
	if err != nil {
		t.Errorf("can't decode: %v", err)
		return
	}

	res1, ok := input.(engine.Res)
	if !ok {
		t.Errorf("input %v is not a Res", res1)
		return
	}
	res2, ok := output.(engine.Res)
	if !ok {
		t.Errorf("output %v is not a Res", res2)
		return
	}
	// this uses the standalone file cmp function
	if err := res1.Cmp(res2); err != nil {
		t.Errorf("the input and output Res values do not match: %+v", err)
	}
}

func TestMiscEncodeDecode3(t *testing.T) {
	var err error

	// encode
	input, err := engine.NewNamedResource("file", "file1")
	if err != nil {
		t.Errorf("can't create: %v", err)
		return
	}
	fileRes := input.(*FileRes) // must not panic
	fileRes.Path = "/tmp/whatever"
	// TODO: add other params/traits/etc here!

	b64, err := engineUtil.ResToB64(input)
	if err != nil {
		t.Errorf("can't encode: %v", err)
		return
	}

	output, err := engineUtil.B64ToRes(b64)
	if err != nil {
		t.Errorf("can't decode: %v", err)
		return
	}

	res1, ok := input.(engine.Res)
	if !ok {
		t.Errorf("input %v is not a Res", res1)
		return
	}
	res2, ok := output.(engine.Res)
	if !ok {
		t.Errorf("output %v is not a Res", res2)
		return
	}
	// this uses the more complete, engine cmp function
	if err := engine.ResCmp(res1, res2); err != nil {
		t.Errorf("the input and output Res values do not match: %+v", err)
	}
}

func TestMiscEncodeDecode4(t *testing.T) {
	var err error
	const (
		Kind = "file"
		Name = "file1"
	)

	// encode
	input, err := engine.NewNamedResource(Kind, Name)
	if err != nil {
		t.Errorf("can't create: %v", err)
		return
	}
	fileRes := input.(*FileRes) // must not panic
	fileRes.Path = "/tmp/whatever"
	// TODO: add other params/traits/etc here!

	b64, err := engineUtil.ResToB64(input)
	if err != nil {
		t.Errorf("can't encode: %v", err)
		return
	}

	output, err := engineUtil.B64ToRes(b64)
	if err != nil {
		t.Errorf("can't decode: %v", err)
		return
	}

	res1, ok := input.(engine.Res)
	if !ok {
		t.Errorf("input %v is not a Res", res1)
		return
	}
	res2, ok := output.(engine.Res)
	if !ok {
		t.Errorf("output %v is not a Res", res2)
		return
	}
	// this uses the more complete, engine cmp function
	if err := engine.ResCmp(res1, res2); err != nil {
		t.Errorf("the input and output Res values do not match: %+v", err)
	}

	// ensure the kind and name are correctly decoded too!
	if kind := res2.Kind(); kind != Kind {
		t.Errorf("the output kind was `%s`, expected `%s`", kind, Kind)
	}
	if name := res2.Name(); name != Name {
		t.Errorf("the output name was `%s`, expected `%s`", name, Name)
	}
}

func TestFileAbsolute1(t *testing.T) {
	// file resource paths should be absolute
	f1 := &FileRes{
		Path: "tmp/a/b", // some relative file
	}
	f2 := &FileRes{
		Path: "tmp/a/b/", // some relative dir
	}
	f3 := &FileRes{
		Path: "tmp", // some short relative file
	}
	if f1.Validate() == nil || f2.Validate() == nil || f3.Validate() == nil {
		t.Errorf("file res should have failed validate")
	}
}

func TestFileSELinuxValidate(t *testing.T) {
	s1 := "system_u:object_r:tmp_t:s0"
	// SELinux should not be allowed with State: absent
	f := &FileRes{
		Path:    "/tmp/foo",
		State:   FileStateAbsent,
		SELinux: &s1,
	}
	if err := f.Validate(); err == nil {
		t.Errorf("param SELinux with State: absent should fail validation")
	}

	// SELinux should be allowed with State: exists
	f = &FileRes{
		Path:    "/tmp/foo",
		State:   FileStateExists,
		SELinux: &s1,
	}
	if err := f.Validate(); err != nil {
		t.Errorf("param SELinux with State: exists should pass validation: %v", err)
	}

	// SELinux as nil should be allowed with State: absent
	f = &FileRes{
		Path:    "/tmp/foo",
		State:   FileStateAbsent,
		SELinux: nil,
	}
	if err := f.Validate(); err != nil {
		t.Errorf("param SELinux as nil with State: absent should pass validation: %v", err)
	}
}

func TestFileSELinuxCmp(t *testing.T) {
	s1 := "system_u:object_r:tmp_t:s0"
	s2 := "system_u:object_r:user_tmp_t:s0"

	f1 := &FileRes{
		Path:    "/tmp/foo",
		SELinux: &s1,
	}
	f2 := &FileRes{
		Path:    "/tmp/foo",
		SELinux: &s1,
	}
	if err := f1.Cmp(f2); err != nil {
		t.Errorf("identical SELinux param should match: %v", err)
	}

	f3 := &FileRes{
		Path:    "/tmp/foo",
		SELinux: &s2,
	}
	if err := f1.Cmp(f3); err == nil {
		t.Errorf("different SELinux param should not match")
	}

	f4 := &FileRes{
		Path:    "/tmp/foo",
		SELinux: nil,
	}
	if err := f1.Cmp(f4); err == nil {
		t.Errorf("param SELinux vs nil SELinux should not match")
	}
}

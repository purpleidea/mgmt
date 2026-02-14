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
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	"github.com/purpleidea/mgmt/pgraph"
)

// TestHTTPServerFileAutoEdge1 tests that an http:file with a Path creates an
// autoedge to the corresponding file resource.
func TestHTTPServerFileAutoEdge1(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile := &FileRes{
		Path: "/tmp/some_file",
	}
	resHTTPFile := &HTTPServerFileRes{
		Path: "/tmp/some_file",
	}
	g.AddVertex(resFile, resHTTPFile)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// one edge: file -> http:file (file must exist before serving)
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge instead of: %d", i)
		return
	}

	expected, err := pgraph.NewGraph("Expected")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}
	edge := &engine.Edge{Name: fmt.Sprintf("%s -> %s", resFile, resHTTPFile)}
	expected.AddEdge(resFile, resHTTPFile, edge)

	vertexCmp := func(v1, v2 pgraph.Vertex) (bool, error) { return v1 == v2, nil }
	edgeCmp := func(e1, e2 pgraph.Edge) (bool, error) { return true, nil }

	if err := expected.GraphCmp(g, vertexCmp, edgeCmp); err != nil {
		t.Errorf("graph doesn't match expected: %s", err)
		return
	}
}

// TestHTTPServerFileAutoEdge2 tests that an http:file with only Data (no Path)
// does not create any autoedges.
func TestHTTPServerFileAutoEdge2(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile := &FileRes{
		Path: "/tmp/some_file",
	}
	resHTTPFile := &HTTPServerFileRes{
		Data: "inline content",
	}
	g.AddVertex(resFile, resHTTPFile)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// no edges: inline data has no file dependency
	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}
}

// TestHTTPServerFileAutoEdge3 tests that an http:file with a directory Path
// creates an autoedge to the corresponding file resource at that directory.
func TestHTTPServerFileAutoEdge3(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resDir := &FileRes{
		Path: "/tmp/data/",
	}
	resHTTPFile := &HTTPServerFileRes{
		Path: "/tmp/data/",
	}
	g.AddVertex(resDir, resHTTPFile)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// one edge: dir -> http:file
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge instead of: %d", i)
		return
	}
}

// TestHTTPServerFileAutoEdge4 tests that no edge is created when the http:file
// Path doesn't match any file resource in the graph. The file resource manages
// "/tmp/other_file" but the http:file serves from "/tmp/some_file", so there is
// no relationship between them.
func TestHTTPServerFileAutoEdge4(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile := &FileRes{
		Path: "/tmp/other_file",
	}
	resHTTPFile := &HTTPServerFileRes{
		Path: "/tmp/some_file",
	}
	g.AddVertex(resFile, resHTTPFile)

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// no edges: paths don't match
	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}
}

// TestHTTPServerFileAutoEdge5 tests that the autoedge works when the file
// resource is identified by Name rather than an explicit Path field. This is
// the real-world mcl pattern where you write file "/tmp/some_file" { ... } and
// the resource Name becomes the path. The file resource's getPath() falls back
// to Name() when Path is empty, so the autoedge should still match.
func TestHTTPServerFileAutoEdge5(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile, err := engine.NewNamedResource("file", "/tmp/some_file")
	if err != nil {
		t.Errorf("error creating file resource: %v", err)
		return
	}
	resHTTPFile := &HTTPServerFileRes{
		Path: "/tmp/some_file",
	}
	g.AddVertex(resFile, resHTTPFile)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// one edge: file -> http:file
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge instead of: %d", i)
		return
	}

	expected, err := pgraph.NewGraph("Expected")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}
	edge := &engine.Edge{Name: fmt.Sprintf("%s -> %s", resFile, resHTTPFile)}
	expected.AddEdge(resFile, resHTTPFile, edge)

	vertexCmp := func(v1, v2 pgraph.Vertex) (bool, error) { return v1 == v2, nil }
	edgeCmp := func(e1, e2 pgraph.Edge) (bool, error) { return true, nil }

	if err := expected.GraphCmp(g, vertexCmp, edgeCmp); err != nil {
		t.Errorf("graph doesn't match expected: %s", err)
		return
	}
}

// TestHTTPServerFileAutoEdge6 tests that the http:file resource does not
// falsely identify itself as a file resource. Three resources are in the graph:
// a file, an http:file serving that file, and an exec that runs a command at
// the same path. The exec should only get an autoedge to the real file
// resource, not to the http:file. If UIDs() incorrectly returned a FileUID, the
// exec would get a spurious second edge to the http:file.
func TestHTTPServerFileAutoEdge6(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile, err := engine.NewNamedResource("file", "/tmp/some_file")
	if err != nil {
		t.Errorf("error creating file resource: %v", err)
		return
	}
	resHTTPFile := &HTTPServerFileRes{
		Path: "/tmp/some_file",
	}
	resExec, err := engine.NewNamedResource("exec", "myexec")
	if err != nil {
		t.Errorf("error creating exec resource: %v", err)
		return
	}
	exc := resExec.(*ExecRes)
	exc.Cmd = "/tmp/some_file"

	g.AddVertex(resFile, resHTTPFile, resExec)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// two edges: file -> http:file, file -> exec
	// there must NOT be an edge from http:file -> exec
	if i := g.NumEdges(); i != 2 {
		t.Errorf("should have 2 edges instead of: %d", i)
		return
	}

	expected, err := pgraph.NewGraph("Expected")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}
	edge1 := &engine.Edge{Name: fmt.Sprintf("%s -> %s", resFile, resHTTPFile)}
	expected.AddEdge(resFile, resHTTPFile, edge1)
	edge2 := &engine.Edge{Name: fmt.Sprintf("%s -> %s", resFile, resExec)}
	expected.AddEdge(resFile, resExec, edge2)

	vertexCmp := func(v1, v2 pgraph.Vertex) (bool, error) { return v1 == v2, nil }
	edgeCmp := func(e1, e2 pgraph.Edge) (bool, error) { return true, nil }

	if err := expected.GraphCmp(g, vertexCmp, edgeCmp); err != nil {
		t.Errorf("graph doesn't match expected: %s", err)
		return
	}
}

// TestHTTPServerFileAutoEdge7 tests that the Filename field (which controls the
// URL path on the HTTP server) does not interfere with autoedge matching. The
// autoedge is based on the disk Path, not the HTTP-visible Filename. Here the
// http:file has Filename "/api/config" but serves from Path "/tmp/some_file",
// and the file resource manages "/tmp/some_file", so one edge should be
// created.
func TestHTTPServerFileAutoEdge7(t *testing.T) {
	g, err := pgraph.NewGraph("TestGraph")
	if err != nil {
		t.Errorf("error creating graph: %v", err)
		return
	}

	resFile := &FileRes{
		Path: "/tmp/some_file",
	}
	resHTTPFile := &HTTPServerFileRes{
		Filename: "/api/config",
		Path:     "/tmp/some_file",
	}
	g.AddVertex(resFile, resHTTPFile)

	if i := g.NumEdges(); i != 0 {
		t.Errorf("should have 0 edges instead of: %d", i)
		return
	}

	debug := testing.Verbose()
	logf := func(format string, v ...interface{}) {
		t.Logf("test: "+format, v...)
	}
	if err := autoedge.AutoEdge(g, debug, logf); err != nil {
		t.Errorf("error running autoedges: %v", err)
		return
	}

	// one edge: file -> http:file
	if i := g.NumEdges(); i != 1 {
		t.Errorf("should have 1 edge instead of: %d", i)
		return
	}
}

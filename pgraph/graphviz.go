// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package pgraph // TODO: this should be a subpackage

import (
	"fmt"
	"html"
	"io/ioutil"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// graphvizDefaultFilter is the default program to run when none are
	// specified.
	graphvizDefaultFilter = "dot"

	ptrLabels     = true
	ptrLabelsSize = 10
)

// Graphvizable is a simple interface to handle the common signature for this
// useful graphviz exec method that is used in debugging. Expect that this
// signature might change if the authors find a different variant more useful.
type Graphvizable interface {

	// ExecGraphviz runs graphviz and stores the result at this absolute
	// path. It (awkwardly) can be used by someone expecting either a
	// filename, or a directory. The filename scenario should be used if you
	// are expecting a single .dot output file. The directory scenario
	// should be used if you are expecting a series of .dot graphs.
	// Directories must end with a trailing slash. A filename passed will
	// get this location overwritten if there is something already there. If
	// the string is empty, it might create a file in a temporary directory
	// somewhere.
	ExecGraphviz(filename string) error
}

// Graphviz adds some visualization features for pgraph.
type Graphviz struct {
	// Name is the display name of the graph. If specified it overrides an
	// amalgamation of any graph names shown.
	Name string

	// Graphs is a collection of graphs to print together and the associated
	// options that should be used to format them during display.
	Graphs map[*Graph]*GraphvizOpts

	// Filter is the graphviz program to run. The default is "dot".
	Filter string

	// Filename is the output location for the graph.
	Filename string

	// Hostname is used as a suffix to the filename when specified.
	Hostname string
}

// graphs returns a list of the graphs in a probably deterministic order.
func (obj *Graphviz) graphs() []*Graph {
	graphs := []*Graph{}
	for g := range obj.Graphs {
		graphs = append(graphs, g)
	}

	sort.Slice(graphs, func(i, j int) bool { return graphs[i].GetName() < graphs[j].GetName() })

	return graphs
}

// name returns a unique name for the combination of graphs.
func (obj *Graphviz) name() string {
	if obj.Name != "" {
		return obj.Name
	}
	names := []string{}
	//for g := range obj.Graphs {
	//	names = append(names, g.GetName())
	//}
	//sort.Strings(names) // deterministic
	for _, g := range obj.graphs() { // deterministic
		names = append(names, g.GetName())
	}
	return strings.Join(names, "|") // arbitrary join character
}

// Text outputs the graph in graphviz format.
// https://en.wikipedia.org/wiki/DOT_%28graph_description_language%29
func (obj *Graphviz) Text() string {
	//digraph g {
	//	label="hello world";
	//	node [shape=box];
	//	A [label="A"];
	//	B [label="B"];
	//	C [label="C"];
	//	D [label="D"];
	//	E [label="E"];
	//	A -> B [label=f];
	//	B -> C [label=g];
	//	D -> E [label=h];
	//}

	str := ""
	name := obj.name()
	str += fmt.Sprintf("digraph \"%s\" {\n", name)
	str += fmt.Sprintf("\tlabel=\"%s\";\n", name)
	//if obj.filter() == "dot" || true {
	str += fmt.Sprintf("\tnewrank=true;\n")
	//}

	//str += "\tnode [shape=box];\n"

	for _, g := range obj.graphs() { // deterministic
		str += g.graphvizBody(obj.Graphs[g])
	}

	str += "}\n"

	return str
}

// Exec writes out the graphviz data and runs the correct graphviz filter
// command.
func (obj *Graphviz) Exec() error {
	filter := ""
	switch obj.Filter {
	case "":
		filter = graphvizDefaultFilter

	case "dot", "neato", "twopi", "circo", "fdp", "sfdp", "patchwork", "osage":
		filter = obj.Filter

	default:
		return fmt.Errorf("invalid graphviz filter selected")
	}

	if obj.Filename == "" {
		return fmt.Errorf("no filename given")
	}

	filename := obj.Filename
	if obj.Hostname != "" {
		filename = fmt.Sprintf("%s@%s", obj.Filename, obj.Hostname)
	}

	// run as a normal user if possible when run with sudo
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))

	if err := ioutil.WriteFile(filename, []byte(obj.Text()), 0644); err != nil {
		return errwrap.Wrapf(err, "error writing to filename")
	}

	if err1 == nil && err2 == nil {
		if err := os.Chown(filename, uid, gid); err != nil {
			return errwrap.Wrapf(err, "error changing file owner")
		}
	}

	path, err := exec.LookPath(filter)
	if err != nil {
		return errwrap.Wrapf(err, "the Graphviz filter is missing")
	}

	out := fmt.Sprintf("%s.png", filename)
	cmd := exec.Command(path, "-Tpng", fmt.Sprintf("-o%s", out), filename)

	if err1 == nil && err2 == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
	}

	if _, err := cmd.Output(); err != nil {
		return errwrap.Wrapf(err, "error writing to image")
	}
	return nil

}

// GraphvizOpts specifies some formatting for each graph.
type GraphvizOpts struct {
	// Style represents the node style string.
	Style string

	// Font represents the node font to use.
	// TODO: implement me
	Font string
}

func (obj *Graph) graphvizBody(opts *GraphvizOpts) string {
	str := ""
	style := ""
	if opts != nil {
		style = opts.Style
	}

	// all in deterministic order
	for _, i := range obj.VerticesSorted() { // reverse paths
		v1 := html.EscapeString(i.String()) // 1st vertex
		if ptrLabels {
			text := fmt.Sprintf("%p", i)
			small := fmt.Sprintf("<FONT POINT-SIZE=\"%d\">%s</FONT>", ptrLabelsSize, text)
			str += fmt.Sprintf("\t\"%p\" [label=<%s<BR />%s>];\n", i, v1, small)
		} else {
			str += fmt.Sprintf("\t\"%p\" [label=<%s>];\n", i, v1)
		}

		vs := []Vertex{}
		for j := range obj.Adjacency()[i] {
			vs = append(vs, j)
		}
		sort.Sort(VertexSlice(vs)) // deterministic order

		for _, j := range vs {
			k := obj.Adjacency()[i][j]
			//v2 := html.EscapeString(j.String()) // 2nd vertex
			e := html.EscapeString(k.String()) // edge
			// use str for clearer output ordering
			//if fmtBoldFn(k) { // TODO: add this sort of formatting
			//	str += fmt.Sprintf("\t\"%s\" -> \"%s\" [label=<%s>,style=bold];\n", i, j, k)
			//} else {
			if false { // XXX: don't need the labels for edges
				text := fmt.Sprintf("%p", k)
				small := fmt.Sprintf("<FONT POINT-SIZE=\"%d\">%s</FONT>", ptrLabelsSize, text)
				str += fmt.Sprintf("\t\"%p\" -> \"%p\" [label=<%s<BR />%s>];\n", i, j, e, small)
			} else {
				if style != "" {
					str += fmt.Sprintf("\t\"%p\" -> \"%p\" [label=<%s>,style=%s];\n", i, j, e, style)
				} else {
					str += fmt.Sprintf("\t\"%p\" -> \"%p\" [label=<%s>];\n", i, j, e)
				}
			}
			//}
		}
	}

	return str
}

// Graphviz outputs the graph in graphviz format.
// https://en.wikipedia.org/wiki/DOT_%28graph_description_language%29
func (obj *Graph) Graphviz() string {
	gv := &Graphviz{
		Graphs: map[*Graph]*GraphvizOpts{
			obj: nil,
		},
	}

	return gv.Text()
}

// ExecGraphviz writes out the graphviz data and runs the correct graphviz
// filter command.
func (obj *Graph) ExecGraphviz(filename string) error {
	gv := &Graphviz{
		Graphs: map[*Graph]*GraphvizOpts{
			obj: nil,
		},

		//Filter:  filter,
		Filename: filename,
		//Hostname: hostname,
	}
	return gv.Exec()
}

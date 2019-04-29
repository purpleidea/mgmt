// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

// Graphviz outputs the graph in graphviz format.
// https://en.wikipedia.org/wiki/DOT_%28graph_description_language%29
func (g *Graph) Graphviz() (out string) {
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
	out += fmt.Sprintf("digraph \"%s\" {\n", g.GetName())
	out += fmt.Sprintf("\tlabel=\"%s\";\n", g.GetName())
	//out += "\tnode [shape=box];\n"
	str := ""
	for i := range g.Adjacency() { // reverse paths
		v1 := strconv.Quote(i.String()) // 1st vertex
		out += fmt.Sprintf("\t\"%p\" [label=%s];\n", i, v1)
		for j := range g.Adjacency()[i] {
			k := g.Adjacency()[i][j]
			//v2 := strconv.Quote(j.String()) // 2nd vertex
			e := strconv.Quote(k.String()) // edge
			// use str for clearer output ordering
			//if fmtBoldFn(k) { // TODO: add this sort of formatting
			//	str += fmt.Sprintf("\t\"%s\" -> \"%s\" [label=\"%s\",style=bold];\n", i, j, k)
			//} else {
			str += fmt.Sprintf("\t\"%p\" -> \"%p\" [label=%s];\n", i, j, e)
			//}
		}
	}
	out += str
	out += "}\n"
	return
}

// ExecGraphviz writes out the graphviz data and runs the correct graphviz
// filter command.
func (g *Graph) ExecGraphviz(program, filename, hostname string) error {

	switch program {
	case "dot", "neato", "twopi", "circo", "fdp":
	default:
		return fmt.Errorf("invalid graphviz program selected")
	}

	if filename == "" {
		return fmt.Errorf("no filename given")
	}

	if hostname != "" {
		filename = fmt.Sprintf("%s@%s", filename, hostname)
	}

	// run as a normal user if possible when run with sudo
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))

	err := ioutil.WriteFile(filename, []byte(g.Graphviz()), 0644)
	if err != nil {
		return fmt.Errorf("error writing to filename")
	}

	if err1 == nil && err2 == nil {
		if err := os.Chown(filename, uid, gid); err != nil {
			return fmt.Errorf("error changing file owner")
		}
	}

	path, err := exec.LookPath(program)
	if err != nil {
		return fmt.Errorf("the Graphviz program is missing")
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
	_, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("error writing to image")
	}
	return nil
}

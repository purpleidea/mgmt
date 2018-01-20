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

package lang // TODO: move this into a sub package of lang/$name?

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"

	errwrap "github.com/pkg/errors"
)

// interpret runs the program and causes a graph generation as a side effect.
// You should not run this on the AST if you haven't previously run the function
// graph engine so that output values have been produced! Type unification is
// another important aspect which needs to have been completed.
func interpret(ast interfaces.Stmt) (*pgraph.Graph, error) {
	output, err := ast.Output() // contains resList, edgeList, etc...
	if err != nil {
		return nil, err
	}

	graph, err := pgraph.NewGraph("interpret") // give graph a default name
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not create new graph")
	}

	var lookup = make(map[string]map[string]resources.Res) // map[kind]map[name]Res
	// build the send/recv mapping; format: map[kind]map[name]map[field]*Send
	var receive = make(map[string]map[string]map[string]*resources.Send)

	for _, res := range output.Resources {
		graph.AddVertex(res)
		kind := res.GetKind()
		name := res.GetName()
		if _, exists := lookup[kind]; !exists {
			lookup[kind] = make(map[string]resources.Res)
			receive[kind] = make(map[string]map[string]*resources.Send)
		}
		if _, exists := receive[kind][name]; !exists {
			receive[kind][name] = make(map[string]*resources.Send)
		}

		if r, exists := lookup[kind][name]; exists { // found same name
			if !r.Compare(res) {
				// TODO: print a diff of the two resources
				return nil, fmt.Errorf("incompatible duplicate resource `%s` found", res)
			}
			// more than one compatible resource exists... we allow
			// duplicates, if they're going to not conflict...
			// XXX: does it matter which one we add to the graph?
		}
		lookup[kind][name] = res // add to temporary lookup table
	}

	for _, e := range output.Edges {
		var v1, v2 resources.Res
		var exists = true
		var m map[string]resources.Res
		var notify = e.Notify

		if m, exists = lookup[e.Kind1]; exists {
			v1, exists = m[e.Name1]
		}
		if !exists {
			return nil, fmt.Errorf("edge cannot find resource kind: %s named: `%s`", e.Kind1, e.Name1)
		}
		if m, exists = lookup[e.Kind2]; exists {
			v2, exists = m[e.Name2]
		}
		if !exists {
			return nil, fmt.Errorf("edge cannot find resource kind: %s named: `%s`", e.Kind2, e.Name2)
		}

		if existingEdge := graph.FindEdge(v1, v2); existingEdge != nil {
			// collate previous Notify signals to this edge with OR
			notify = notify || (existingEdge.(*resources.Edge)).Notify
		}

		edge := &resources.Edge{
			Name:   fmt.Sprintf("%s -> %s", v1, v2),
			Notify: notify,
		}
		graph.AddEdge(v1, v2, edge) // identical duplicates are ignored

		// send recv
		if (e.Send == "") != (e.Recv == "") { // xor
			return nil, fmt.Errorf("you must specify both send/recv fields or neither")
		}
		if e.Send == "" || e.Recv == "" { // is there send/recv to do or not?
			continue
		}

		// check for pre-existing send/recv at this key
		if existingSend, exists := receive[e.Kind2][e.Name2][e.Recv]; exists {
			// ignore identical duplicates
			// TODO: does this safe ignore work with duplicate compatible resources?
			if existingSend.Res != v1 || existingSend.Key != e.Send {
				return nil, fmt.Errorf("resource kind: %s named: `%s` already receives on `%s`", e.Kind2, e.Name2, e.Recv)
			}
		}

		// store mapping for later
		receive[e.Kind2][e.Name2][e.Recv] = &resources.Send{Res: v1, Key: e.Send}
	}

	// we need to first build up a map of all the resources handles, because
	// we don't know which order send/recv pairs will arrive in, and we need
	// to ensure the right pointer exists before we reference it... finally,
	// we build up a list of send/recv mappings to ensure we don't overwrite
	// pre-existing mappings, so we can now set them all at once at the end!

	// TODO: do this in a deterministic order
	for kind, x := range receive {
		for name, recv := range x {
			lookup[kind][name].SetRecv(recv) // set it!
		}
	}

	// ensure that we have a DAG!
	if _, err := graph.TopologicalSort(); err != nil {
		// TODO: print information on the cycles
		return nil, errwrap.Wrapf(err, "resource graph has cycles")
	}

	return graph, nil
}

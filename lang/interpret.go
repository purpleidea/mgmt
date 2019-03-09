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

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"

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

	var lookup = make(map[string]map[string]engine.Res) // map[kind]map[name]Res
	// build the send/recv mapping; format: map[kind]map[name]map[field]*Send
	var receive = make(map[string]map[string]map[string]*engine.Send)

	for _, res := range output.Resources {
		kind := res.Kind()
		name := res.Name()
		if _, exists := lookup[kind]; !exists {
			lookup[kind] = make(map[string]engine.Res)
			receive[kind] = make(map[string]map[string]*engine.Send)
		}
		if _, exists := receive[kind][name]; !exists {
			receive[kind][name] = make(map[string]*engine.Send)
		}

		if r, exists := lookup[kind][name]; exists { // found same name
			// if the resources support the compatibility API, then
			// we can attempt to merge them intelligently...
			r1, ok1 := r.(engine.CompatibleRes)
			r2, ok2 := res.(engine.CompatibleRes)
			if ok1 && ok2 {
				if err := engine.AdaptCmp(r1, r2); err != nil {
					// TODO: print a diff of the two resources
					return nil, errwrap.Wrapf(err, "incompatible duplicate resource `%s` found", res)
				}
				merged, err := engine.ResMerge(r1, r2)
				if err != nil {
					return nil, errwrap.Wrapf(err, "could not merge duplicate resources")
				}

				lookup[kind][name] = merged
				// they match here, we don't need to test below!
				continue
			}

			if err := engine.ResCmp(r, res); err != nil {
				// TODO: print a diff of the two resources
				return nil, errwrap.Wrapf(err, "inequivalent duplicate resource `%s` found", res)
			}
			// more than one identical resource exists. we can allow
			// duplicates, if they're not going to conflict... since
			// it was identical, we leave the earlier version in the
			// graph since they're exactly equivalent anyways.
			// TODO: does it matter which one we add to the graph?
			// currently we add the first one that was found...
			continue
		}
		lookup[kind][name] = res // add to temporary lookup table
		//graph.AddVertex(res) // do this below once this table is final
	}

	// ensure all the vertices exist...
	for _, m := range lookup {
		for _, res := range m {
			graph.AddVertex(res)
		}
	}

	for _, e := range output.Edges {
		var v1, v2 engine.Res
		var exists bool
		var m map[string]engine.Res
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
			notify = notify || (existingEdge.(*engine.Edge)).Notify
		}

		edge := &engine.Edge{
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
				return nil, fmt.Errorf("resource: `%s` has duplicate receive on: `%s` param", engine.Repr(e.Kind2, e.Name2), e.Recv)
			}
		}

		res1, ok := v1.(engine.SendableRes)
		if !ok {
			return nil, fmt.Errorf("cannot send from resource: %s", engine.Stringer(v1))
		}
		res2, ok := v2.(engine.RecvableRes)
		if !ok {
			return nil, fmt.Errorf("cannot recv to resource: %s", engine.Stringer(v2))
		}

		if err := engineUtil.StructFieldCompat(res1.Sends(), e.Send, res2, e.Recv); err != nil {
			return nil, errwrap.Wrapf(err, "cannot send/recv from %s.%s to %s.%s", engine.Stringer(v1), e.Send, engine.Stringer(v2), e.Recv)
		}

		// store mapping for later
		receive[e.Kind2][e.Name2][e.Recv] = &engine.Send{Res: res1, Key: e.Send}
	}

	// we need to first build up a map of all the resources handles, because
	// we don't know which order send/recv pairs will arrive in, and we need
	// to ensure the right pointer exists before we reference it... finally,
	// we build up a list of send/recv mappings to ensure we don't overwrite
	// pre-existing mappings, so we can now set them all at once at the end!

	// TODO: do this in a deterministic order
	for kind, x := range receive {
		for name, recv := range x {
			if len(recv) == 0 { // skip empty maps from allocation!
				continue
			}
			r := lookup[kind][name]
			res, ok := r.(engine.RecvableRes)
			if !ok {
				return nil, fmt.Errorf("cannot recv to resource: %s", engine.Repr(kind, name))
			}
			res.SetRecv(recv) // set it!
		}
	}

	// ensure that we have a DAG!
	if _, err := graph.TopologicalSort(); err != nil {
		// TODO: print information on the cycles
		return nil, errwrap.Wrapf(err, "resource graph has cycles")
	}

	return graph, nil
}

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

// Package interpret contains the implementation of the actual interpret
// function that takes an AST and returns a resource graph.
package interpret

import (
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// Interpreter is a base struct for handling the Interpret operation. There is
// nothing stateful here, you don't need to preserve this between runs.
type Interpreter struct {
	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})

	// lookup stores the resources found by kind and name. It doesn't store
	// any resources which are hidden since those could have duplicates.
	// format: map[kind]map[name]Res
	lookup map[engine.ResPtrUID]engine.Res

	// lookupHidden stores the hidden resources found by kind and name. It
	// doesn't store any normal resources which are not hidden.
	// format formerly: map[kind]map[name]Res
	lookupHidden map[engine.ResPtrUID][]engine.Res

	// receive doesn't need a special extension for hidden resources since
	// they can't send, only recv, and senders can't have incompatible dupes
	// format formerly: map[kind]map[name]map[field]*Send
	receive map[engine.ResPtrUID]map[string]*engine.Send

	// export tracks the unique combinations we export. (kind, name, host)
	export map[engine.ResDelete]struct{}
}

// Interpret runs the program and outputs a generated resource graph. It
// requires an AST, and the table of values required to populate that AST. Type
// unification, and earlier steps should obviously be run first so that you can
// actually get a useful resource graph out of this instead of an error!
// XXX: add a ctx?
func (obj *Interpreter) Interpret(ast interfaces.Stmt, table interfaces.Table) (*pgraph.Graph, error) {
	obj.Logf("interpreting...")

	// build the kind,name -> res mapping
	obj.lookup = make(map[engine.ResPtrUID]engine.Res)
	obj.lookupHidden = make(map[engine.ResPtrUID][]engine.Res)
	// build the send/recv mapping
	obj.receive = make(map[engine.ResPtrUID]map[string]*engine.Send)
	// build the exports
	obj.export = make(map[engine.ResDelete]struct{})

	// Remember that if a resource is "Hidden", then make sure it is NOT
	// sending to anyone, since it would never produce a value. It can
	// receive values, since those might be used during export.
	//
	// Remember that if a resource is "Hidden", then it may exist alongside
	// another resource with the same kind+name without triggering the
	// "inequivalent duplicate resource" style of errors. Of course multiple
	// hidden resources with the same kind+name may also exist
	// simultaneously, just keep in mind that it means that an edge pointing
	// to a particular kind+name now actually may point to more than one!
	//
	// This is needed because of two reasons: (1) because a regular resource
	// will likely never be compatible with a "Hidden" and "Exported"
	// resource because one resource might have the Meta:hidden and
	// Meta:export params and one might not; (2) because you may wish to
	// have two different hidden resources of different params which export
	// to different hosts, which means they would likely not be compatible.
	//
	// Since we can have more than one "Hidden" and "Exported" resource with
	// the same name and kind, it's important that we don't export that data
	// to the same (kind, name, host) location since we'd have multiple
	// writers to the same key in our World store. We could consider
	// checking for compatibility, but that's more difficult to achieve. The
	// "any" host is treated as a special key, which punts this duplicate
	// problem to being a collection problem. (Which could happen with two
	// different hosts each exporting a different value to a single host.)
	//
	// Remember that the resource graph that this function returns, may now
	// contain two or more identically named kind+name resources, if at
	// least one of them is "Hidden". If they are entirely identical, then
	// it's acceptable to merge them. They may _not_ be merged with the
	// CompatibleRes API, since on resource "collection" a param may be
	// changed which could conceivably be incompatible with how we ran the
	// AdaptCmp API when we merged them.

	output, err := ast.Output(table) // contains resList, edgeList, etc...
	if err != nil {
		return nil, err
	}

	graph, err := pgraph.NewGraph("interpret") // give graph a default name
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not create new graph")
	}

	for _, res := range output.Resources {
		kind := res.Kind()
		name := res.Name()
		meta := res.MetaParams()
		ruid := engine.ResPtrUID{
			Kind: kind,
			Name: name,
		}

		for _, host := range meta.Export {
			uid := engine.ResDelete{
				Kind: kind,
				Name: name,
				Host: host,
			}
			if _, exists := obj.export[uid]; exists {
				return nil, fmt.Errorf("duplicate export: %s to %s", res, host)
			}
			obj.export[uid] = struct{}{}
		}

		if meta.Hidden {
			rs := obj.lookupHidden[ruid]
			if len(rs) > 0 {
				// We only need to check against the last added
				// resource since this should be commutative,
				// and as we add more they check themselves in.
				r := rs[len(rs)-1]

				// XXX: If we want to check against the regular
				// resources in obj.lookup, then do it here.

				// If they're different, then we deduplicate.
				if err := engine.ResCmp(r, res); err == nil {
					continue
				}
			}

			// add to temporary lookup table
			obj.lookupHidden[ruid] = append(obj.lookupHidden[ruid], res)
			continue
		}

		if r, exists := obj.lookup[ruid]; exists { // found same name
			// XXX: If we want to check against the special hidden
			// resources in obj.lookupHidden, then do it here.

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

				obj.lookup[ruid] = merged
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
		obj.lookup[ruid] = res // add to temporary lookup table
		//graph.AddVertex(res) // do this below once this table is final
	}

	// ensure all the vertices exist...
	for _, res := range obj.lookup {
		graph.AddVertex(res)
	}
	for _, rs := range obj.lookupHidden {
		for _, res := range rs {
			graph.AddVertex(res)
		}
	}

	for _, edge := range output.Edges {
		v1s := obj.lookupAll(edge.Kind1, edge.Name1)
		if len(v1s) == 0 {
			return nil, fmt.Errorf("edge cannot find resource kind: %s named: `%s`", edge.Kind1, edge.Name1)
		}
		v2s := obj.lookupAll(edge.Kind2, edge.Name2)
		if len(v2s) == 0 {
			return nil, fmt.Errorf("edge cannot find resource kind: %s named: `%s`", edge.Kind2, edge.Name2)
		}

		// Make edges pair wise between each two. Normally these loops
		// only have one iteration each unless we have Hidden resources.
		for _, v1 := range v1s {
			for _, v2 := range v2s {
				e := obj.makeEdge(graph, v1, v2, edge)
				graph.AddEdge(v1, v2, e) // identical duplicates are ignored
			}
		}

		// send recv
		if (edge.Send == "") != (edge.Recv == "") { // xor
			return nil, fmt.Errorf("you must specify both send/recv fields or neither")
		}
		if edge.Send == "" || edge.Recv == "" { // is there send/recv to do or not?
			continue
		}

		for _, v1 := range v1s {
			for _, v2 := range v2s {
				if err := obj.makeSendRecv(v1, v2, edge); err != nil {
					return nil, err
				}
			}
		}
	}

	// we need to first build up a map of all the resources handles, because
	// we don't know which order send/recv pairs will arrive in, and we need
	// to ensure the right pointer exists before we reference it... finally,
	// we build up a list of send/recv mappings to ensure we don't overwrite
	// pre-existing mappings, so we can now set them all at once at the end!

	// TODO: do this in a deterministic order
	for st, recv := range obj.receive {
		kind := st.Kind
		name := st.Name

		if len(recv) == 0 { // skip empty maps from allocation!
			continue
		}
		if r := obj.lookupRes(kind, name); r != nil {
			res, ok := r.(engine.RecvableRes)
			if !ok {
				return nil, fmt.Errorf("cannot recv to resource: %s", engine.Repr(kind, name))
			}
			res.SetRecv(recv) // set it!
		}

		// hidden
		for _, r := range obj.lookupHiddenRes(kind, name) {
			res, ok := r.(engine.RecvableRes)
			if !ok {
				return nil, fmt.Errorf("cannot recv to resource: %s", engine.Repr(kind, name))
			}
			res.SetRecv(recv) // set it!
		}
	}

	// ensure that we have a DAG!
	if _, err := graph.TopologicalSort(); err != nil {
		errNotAcyclic, ok := err.(*pgraph.ErrNotAcyclic)
		if !ok {
			return nil, err // programming error
		}
		obj.Logf("%s", err)
		for _, vertex := range errNotAcyclic.Cycle {
			obj.Logf("* %s", vertex)
		}
		return nil, errwrap.Wrapf(err, "resource graph has cycles")
	}

	return graph, nil
}

// lookupRes is a simple helper function. Returns nil if not found.
func (obj *Interpreter) lookupRes(kind, name string) engine.Res {
	ruid := engine.ResPtrUID{
		Kind: kind,
		Name: name,
	}
	res, exists := obj.lookup[ruid]
	if !exists {
		return nil
	}

	return res
}

// lookupHiddenRes is a simple helper function. Returns any found.
func (obj *Interpreter) lookupHiddenRes(kind, name string) []engine.Res {
	ruid := engine.ResPtrUID{
		Kind: kind,
		Name: name,
	}
	res, exists := obj.lookupHidden[ruid]
	if !exists {
		return nil
	}

	return res
}

// lookupAll is a simple helper function. Returns any found.
func (obj *Interpreter) lookupAll(kind, name string) []pgraph.Vertex {
	vs := []pgraph.Vertex{}

	if r := obj.lookupRes(kind, name); r != nil {
		vs = append(vs, r)
	}

	for _, r := range obj.lookupHiddenRes(kind, name) {
		vs = append(vs, r)
	}

	return vs
}

// makeEdge is a simple helper function.
func (obj *Interpreter) makeEdge(graph *pgraph.Graph, v1, v2 pgraph.Vertex, edge *interfaces.Edge) *engine.Edge {
	var notify = edge.Notify

	if existingEdge := graph.FindEdge(v1, v2); existingEdge != nil {
		// collate previous Notify signals to this edge with OR
		notify = notify || (existingEdge.(*engine.Edge)).Notify
	}

	return &engine.Edge{
		Name:   fmt.Sprintf("%s -> %s", v1, v2),
		Notify: notify,
	}
}

// makeSendRecv is a simple helper function.
func (obj *Interpreter) makeSendRecv(v1, v2 pgraph.Vertex, edge *interfaces.Edge) error {
	ruid := engine.ResPtrUID{
		Kind: edge.Kind2,
		Name: edge.Name2,
	}

	if _, exists := obj.receive[ruid]; !exists {
		obj.receive[ruid] = make(map[string]*engine.Send)
	}

	// check for pre-existing send/recv at this key
	if existingSend, exists := obj.receive[ruid][edge.Recv]; exists {
		// ignore identical duplicates
		// TODO: does this safe ignore work with duplicate compatible resources?
		if existingSend.Res != v1 || existingSend.Key != edge.Send {
			return fmt.Errorf("resource: `%s` has duplicate receive on: `%s` param", engine.Repr(edge.Kind2, edge.Name2), edge.Recv)
		}
	}

	if res, ok := v1.(engine.Res); ok && res.MetaParams().Hidden && edge.Send != "" {
		return fmt.Errorf("cannot send from hidden resource: %s", engine.Stringer(res))
	}

	res1, ok := v1.(engine.SendableRes)
	if !ok {
		return fmt.Errorf("cannot send from resource: %s", engine.Stringer(res1))
	}
	res2, ok := v2.(engine.RecvableRes)
	if !ok {
		return fmt.Errorf("cannot recv to resource: %s", engine.Stringer(res2))
	}

	if err := engineUtil.StructFieldCompat(res1.Sends(), edge.Send, res2, edge.Recv); err != nil {
		return errwrap.Wrapf(err, "cannot send/recv from %s.%s to %s.%s", engine.Stringer(res1), edge.Send, engine.Stringer(res2), edge.Recv)
	}

	// XXX: Not doing this for now, see the interface for more information.
	// TODO: We could instead pass in edge.Send so it would know precisely!
	//res1.SendSetActive(true) // tell it that it will be sending (optimization)

	// store mapping for later
	obj.receive[ruid][edge.Recv] = &engine.Send{Res: res1, Key: edge.Send}

	return nil
}

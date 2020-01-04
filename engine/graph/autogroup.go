// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package graph

import (
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph/autogroup"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// AutoGroup runs the auto grouping on the loaded graph.
func (obj *Engine) AutoGroup(ag engine.AutoGrouper) error {
	if obj.nextGraph == nil {
		return fmt.Errorf("there is no active graph to autogroup")
	}

	logf := func(format string, v ...interface{}) {
		obj.Logf("autogroup: "+format, v...)
	}

	// wrap ag with our own vertexCmp, vertexMerge and edgeMerge
	wrapped := &wrappedGrouper{
		AutoGrouper: ag, // pass in the existing autogrouper
	}

	if err := autogroup.AutoGroup(wrapped, obj.nextGraph, obj.Debug, logf); err != nil {
		return errwrap.Wrapf(err, "autogrouping failed")
	}

	return nil
}

// wrappedGrouper is an autogrouper which adds our own Cmp and Merge functions
// on top of the desired AutoGrouper that was specified.
type wrappedGrouper struct {
	engine.AutoGrouper // anonymous interface
}

func (obj *wrappedGrouper) Name() string {
	return fmt.Sprintf("wrappedGrouper: %s", obj.AutoGrouper.Name())
}

func (obj *wrappedGrouper) VertexCmp(v1, v2 pgraph.Vertex) error {
	// call existing vertexCmp first
	if err := obj.AutoGrouper.VertexCmp(v1, v2); err != nil {
		return err
	}

	r1, ok := v1.(engine.GroupableRes)
	if !ok {
		return fmt.Errorf("v1 is not a GroupableRes")
	}
	r2, ok := v2.(engine.GroupableRes)
	if !ok {
		return fmt.Errorf("v2 is not a GroupableRes")
	}

	// Some resources of different kinds can now group together!
	//if r1.Kind() != r2.Kind() { // we must group similar kinds
	//	return fmt.Errorf("the two resources aren't the same kind")
	//}
	// someone doesn't want to group!
	if r1.AutoGroupMeta().Disabled || r2.AutoGroupMeta().Disabled {
		return fmt.Errorf("one of the autogroup flags is false")
	}

	if r1.IsGrouped() { // already grouped!
		return fmt.Errorf("already grouped")
	}
	if len(r2.GetGroup()) > 0 { // already has children grouped!
		return fmt.Errorf("already has groups")
	}
	if err := r1.GroupCmp(r2); err != nil { // resource groupcmp failed!
		return errwrap.Wrapf(err, "the GroupCmp failed")
	}

	return nil
}

func (obj *wrappedGrouper) VertexMerge(v1, v2 pgraph.Vertex) (v pgraph.Vertex, err error) {
	r1, ok := v1.(engine.GroupableRes)
	if !ok {
		return nil, fmt.Errorf("v1 is not a GroupableRes")
	}
	r2, ok := v2.(engine.GroupableRes)
	if !ok {
		return nil, fmt.Errorf("v2 is not a GroupableRes")
	}

	if err = r1.GroupRes(r2); err != nil { // GroupRes skips stupid groupings
		return // return early on error
	}

	// merging two resources into one should yield the sum of their semas
	if semas := r2.MetaParams().Sema; len(semas) > 0 {
		r1.MetaParams().Sema = append(r1.MetaParams().Sema, semas...)
		r1.MetaParams().Sema = util.StrRemoveDuplicatesInList(r1.MetaParams().Sema)
	}

	return // success or fail, and no need to merge the actual vertices!
}

func (obj *wrappedGrouper) EdgeMerge(e1, e2 pgraph.Edge) pgraph.Edge {
	e1x, ok := e1.(*engine.Edge)
	if !ok {
		return e2 // just return something to avoid needing to error
	}
	e2x, ok := e2.(*engine.Edge)
	if !ok {
		return e1 // just return something to avoid needing to error
	}

	// TODO: should we merge the edge.Notify or edge.refresh values?
	edge := &engine.Edge{
		Notify: e1x.Notify || e2x.Notify, // TODO: should we merge this?
	}
	refresh := e1x.Refresh() || e2x.Refresh() // TODO: should we merge this?
	edge.SetRefresh(refresh)

	return edge
}

// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package resources

import (
	"fmt"

	"github.com/purpleidea/mgmt/pgraph"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

func init() {
	RegisterResource("graph", func() Res { return &GraphRes{} })
}

// GraphRes is a resource that recursively runs a sub graph of resources.
// TODO: should we name this SubGraphRes instead?
// TODO: we could also flatten "sub graphs" into the main graph to avoid this,
// and this could even be done with a graph transformation called flatten,
// similar to where autogroup and autoedges run.
// XXX: this resource is not complete, and hasn't even been tested
type GraphRes struct {
	BaseRes `yaml:",inline"`
	Graph   *pgraph.Graph `yaml:"graph"` // TODO: how do we suck in a graph via yaml?

	initCount int // number of successfully initialized resources
}

// GraphUID is a unique representation for a GraphRes object.
type GraphUID struct {
	BaseUID
	//foo string // XXX: not implemented
}

// Default returns some sensible defaults for this resource.
func (obj *GraphRes) Default() Res {
	return &GraphRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate the params and sub resources that are passed to GraphRes.
func (obj *GraphRes) Validate() error {
	var err error
	for _, v := range obj.Graph.VerticesSorted() { // validate everyone
		if e := VtoR(v).Validate(); err != nil {
			err = multierr.Append(err, e) // list of errors
		}
	}
	if err != nil {
		return errwrap.Wrapf(err, "could not Validate() graph")
	}

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *GraphRes) Init() error {
	// Loop through each vertex and initialize it, but keep track of how far
	// we've succeeded, because on failure we'll stop and prepare to reverse
	// through from there running the Close operation on each vertex that we
	// previously did an Init on. The engine always ensures that we run this
	// with a 1-1 relationship between Init and Close, so we must do so too.
	for i, v := range obj.Graph.VerticesSorted() { // deterministic order!
		obj.initCount = i + 1 // store the number that we tried to init
		if err := VtoR(v).Init(); err != nil {
			return errwrap.Wrapf(err, "could not Init() graph")
		}
	}

	return obj.BaseRes.Init() // call base init, b/c we're overrriding
}

// Close runs some cleanup code for this resource.
func (obj *GraphRes) Close() error {
	// The idea is to Close anything we did an Init on including the BaseRes
	// methods which are not guaranteed to be safe if called multiple times!
	var err error
	vertices := obj.Graph.VerticesSorted() // deterministic order!
	last := obj.initCount - 1              // index of last vertex we did init on
	for i := range vertices {
		v := vertices[last-i] // go through in reverse

		// if we hit this condition, we haven't been able to get through
		// the entire list of vertices that we'd have liked to, on init!
		if obj.initCount == 0 {
			// if we get here, we exit without calling BaseRes.Close
			// because the matching BaseRes.Init did not get called!
			return errwrap.Wrapf(err, "could not Close() partial graph")
			//break
		}

		obj.initCount-- // count to avoid closing one that didn't init!
		// try to close everyone that got an init, don't stop suddenly!
		if e := VtoR(v).Close(); e != nil {
			err = multierr.Append(err, e) // list of errors
		}
	}

	// call base close, b/c we're overriding
	if e := obj.BaseRes.Close(); err == nil {
		err = e
	} else if e != nil {
		err = multierr.Append(err, e) // list of errors
	}
	// this returns nil if err is nil
	return errwrap.Wrapf(err, "could not Close() graph")
}

// Watch is the primary listener for this resource and it outputs events.
// XXX: should this use mgraph.Start/Pause? if so then what does CheckApply do?
// XXX: we should probably refactor the core engine to make this work, which
// will hopefully lead us to a more elegant core that is easier to understand
func (obj *GraphRes) Watch() error {

	return fmt.Errorf("Not implemented")
}

// CheckApply method for Graph resource.
// XXX: not implemented
func (obj *GraphRes) CheckApply(apply bool) (bool, error) {

	return false, fmt.Errorf("Not implemented")

}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *GraphRes) UIDs() []ResUID {
	x := &GraphUID{
		BaseUID: BaseUID{
			Name: obj.GetName(),
			Kind: obj.GetKind(),
		},
		//foo: obj.foo, // XXX: not implemented
	}
	uids := []ResUID{}
	for _, v := range obj.Graph.VerticesSorted() {
		uids = append(uids, VtoR(v).UIDs()...)
	}
	return append([]ResUID{x}, uids...)
}

// XXX: hook up the autogrouping magic!

// Compare two resources and return if they are equivalent.
func (obj *GraphRes) Compare(r Res) bool {
	// we can only compare GraphRes to others of the same resource kind
	res, ok := r.(*GraphRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) {
		return false
	}
	if obj.Name != res.Name {
		return false
	}

	//if obj.Foo != res.Foo { // XXX: not implemented
	//	return false
	//}
	// compare the structure of the two graphs...
	vertexCmpFn := func(v1, v2 pgraph.Vertex) (bool, error) {
		if v1.String() == "" || v2.String() == "" {
			return false, fmt.Errorf("oops, empty vertex")
		}
		return VtoR(v1).Compare(VtoR(v2)), nil
	}

	edgeCmpFn := func(e1, e2 pgraph.Edge) (bool, error) {
		if e1.String() == "" || e2.String() == "" {
			return false, fmt.Errorf("oops, empty edge")
		}
		edge1 := e1.(*Edge) // panic if wrong
		edge2 := e2.(*Edge) // panic if wrong
		return edge1.Compare(edge2), nil
	}
	if err := obj.Graph.GraphCmp(res.Graph, vertexCmpFn, edgeCmpFn); err != nil {
		return false
	}

	// compare individual elements in structurally equivalent graphs
	// TODO: is this redundant with the GraphCmp?
	g1 := obj.Graph.VerticesSorted()
	g2 := res.Graph.VerticesSorted()
	for i, v1 := range g1 {
		v2 := g2[i]
		if !VtoR(v1).Compare(VtoR(v2)) {
			return false
		}
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *GraphRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes GraphRes // indirection to avoid infinite recursion

	def := obj.Default()       // get the default
	res, ok := def.(*GraphRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to GraphRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = GraphRes(raw) // restore from indirection with type conversion!
	return nil
}

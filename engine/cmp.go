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

package engine

import (
	"fmt"

	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// ResCmp compares two resources by checking multiple aspects. This is the main
// entry point for running all the compare steps on two resources. This code is
// very similar to AdaptCmp.
func ResCmp(r1, r2 Res) error {
	if r1.Kind() != r2.Kind() {
		return fmt.Errorf("kind differs")
	}
	if r1.Name() != r2.Name() {
		return fmt.Errorf("name differs")
	}

	if err := r1.Cmp(r2); err != nil {
		return err
	}

	// TODO: do we need to compare other traits/metaparams?

	m1 := r1.MetaParams()
	m2 := r2.MetaParams()
	if (m1 == nil) != (m2 == nil) { // xor
		return fmt.Errorf("meta params differ")
	}
	if m1 != nil && m2 != nil {
		if err := m1.Cmp(m2); err != nil {
			return err
		}
	}

	r1x, ok1 := r1.(RefreshableRes)
	r2x, ok2 := r2.(RefreshableRes)
	if ok1 != ok2 {
		return fmt.Errorf("refreshable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1x.Refresh() != r2x.Refresh() {
			return fmt.Errorf("refresh differs")
		}
	}

	// compare meta params for resources with auto edges
	r1e, ok1 := r1.(EdgeableRes)
	r2e, ok2 := r2.(EdgeableRes)
	if ok1 != ok2 {
		return fmt.Errorf("edgeable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1e.AutoEdgeMeta().Cmp(r2e.AutoEdgeMeta()) != nil {
			return fmt.Errorf("autoedge differs")
		}
	}

	// compare meta params for resources with auto grouping
	r1g, ok1 := r1.(GroupableRes)
	r2g, ok2 := r2.(GroupableRes)
	if ok1 != ok2 {
		return fmt.Errorf("groupable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1g.AutoGroupMeta().Cmp(r2g.AutoGroupMeta()) != nil {
			return fmt.Errorf("autogroup differs")
		}

		// if resources are grouped, are the groups the same?
		if i, j := r1g.GetGroup(), r2g.GetGroup(); len(i) != len(j) {
			return fmt.Errorf("autogroup groups differ")
		} else if len(i) > 0 { // trick the golinter

			// Sort works with Res, so convert the lists to that
			iRes := []Res{}
			for _, r := range i {
				res := r.(Res)
				iRes = append(iRes, res)
			}
			jRes := []Res{}
			for _, r := range j {
				res := r.(Res)
				jRes = append(jRes, res)
			}

			ix, jx := Sort(iRes), Sort(jRes) // now sort :)
			for k := range ix {
				// compare sub resources
				if err := ResCmp(ix[k], jx[k]); err != nil {
					//fmt.Printf("bad Cmp: %+v <> %+v for: %+v <> %+v err: %+v\n", r1, r2, ix[k], jx[k], err)
					return err
				}
			}
		}
	}

	r1r, ok1 := r1.(RecvableRes)
	r2r, ok2 := r2.(RecvableRes)
	if ok1 != ok2 {
		return fmt.Errorf("recvable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		v1 := r1r.Recv()
		v2 := r2r.Recv()

		// XXX: Our Send/Recv in the lib/main.go doesn't seem to be
		// pulling this in, so this always compares differently. We can
		// comment it out for now, since it's not too consequential.
		// XXX: Find out what the issue is and fix it for here and send.
		// XXX: The below errors are commented out until this is fixed.
		if (v1 == nil) != (v2 == nil) { // xor
			//return fmt.Errorf("recv params differ")
		}
		if v1 != nil && v2 != nil {
			if len(v1) != len(v2) {
				//return fmt.Errorf("recv param lengths differ")
			}
			for key, send1 := range v1 { // map[string]*engine.Send
				send2, exists := v2[key]
				if !exists {
					//return fmt.Errorf("recv param key %s doesn't exist", key)
				}
				if (send1 == nil) != (send2 == nil) { // xor
					//return fmt.Errorf("recv param key %s send differs", key)
				}
				if send1 != nil && send2 != nil && send1.Key != send2.Key {
					//return fmt.Errorf("recv param key %s send key differs (%v != %v)", key, send1.Key, send2.Key)
				}
			}
		}
	}

	r1s, ok1 := r1.(SendableRes)
	r2s, ok2 := r2.(SendableRes)
	if ok1 != ok2 {
		return fmt.Errorf("sendable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		s1 := r1s.Sent()
		s2 := r2s.Sent()

		// XXX: Our Send/Recv in the lib/main.go doesn't seem to be
		// pulling this in, so this always compares differently. We can
		// comment it out for now, since it's not too consequential.
		// XXX: Find out what the issue is and fix it for here and recv.
		// XXX: The below errors are commented out until this is fixed.
		if (s1 == nil) != (s2 == nil) { // xor
			//return fmt.Errorf("send params differ")
		}
		if s1 != nil && s2 != nil {
			// TODO: reflect.DeepEqual?
			//return fmt.Errorf("send params exist")
		}
	}

	// compare meta params for resources with reversible traits
	r1v, ok1 := r1.(ReversibleRes)
	r2v, ok2 := r2.(ReversibleRes)
	if ok1 != ok2 {
		return fmt.Errorf("reversible differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1v.ReversibleMeta().Cmp(r2v.ReversibleMeta()) != nil {
			return fmt.Errorf("reversible differs")
		}
	}

	return nil
}

// AdaptCmp compares two resources by checking multiple aspects. This is the
// main entry point for running all the compatible compare steps on two
// resources. This code is very similar to ResCmp.
func AdaptCmp(r1, r2 CompatibleRes) error {
	if r1.Kind() != r2.Kind() {
		return fmt.Errorf("kind differs")
	}
	if r1.Name() != r2.Name() {
		return fmt.Errorf("name differs")
	}

	// run `Adapts` instead of `Cmp`
	if err := r1.Adapts(r2); err != nil {
		return err
	}

	// TODO: do we need to compare other traits/metaparams?

	m1 := r1.MetaParams()
	m2 := r2.MetaParams()
	if (m1 == nil) != (m2 == nil) { // xor
		return fmt.Errorf("meta params differ")
	}
	if m1 != nil && m2 != nil {
		if err := m1.Cmp(m2); err != nil {
			return err
		}
	}

	// we don't need to compare refresh, since those can always be merged...

	// compare meta params for resources with auto edges
	r1e, ok1 := r1.(EdgeableRes)
	r2e, ok2 := r2.(EdgeableRes)
	if ok1 != ok2 {
		return fmt.Errorf("edgeable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1e.AutoEdgeMeta().Cmp(r2e.AutoEdgeMeta()) != nil {
			return fmt.Errorf("autoedge differs")
		}
	}

	// compare meta params for resources with auto grouping
	r1g, ok1 := r1.(GroupableRes)
	r2g, ok2 := r2.(GroupableRes)
	if ok1 != ok2 {
		return fmt.Errorf("groupable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1g.AutoGroupMeta().Cmp(r2g.AutoGroupMeta()) != nil {
			return fmt.Errorf("autogroup differs")
		}

		// if resources are grouped, are the groups the same?
		if i, j := r1g.GetGroup(), r2g.GetGroup(); len(i) != len(j) {
			return fmt.Errorf("autogroup groups differ")
		} else if len(i) > 0 { // trick the golinter

			// Sort works with Res, so convert the lists to that
			iRes := []Res{}
			for _, r := range i {
				res := r.(Res)
				iRes = append(iRes, res)
			}
			jRes := []Res{}
			for _, r := range j {
				res := r.(Res)
				jRes = append(jRes, res)
			}

			ix, jx := Sort(iRes), Sort(jRes) // now sort :)
			for k := range ix {
				// compare sub resources
				// TODO: should we use AdaptCmp here?
				// TODO: how would they run `Merge` ? (we don't)
				// this code path will probably not run, because
				// it is called in the lang before autogrouping!
				if err := ResCmp(ix[k], jx[k]); err != nil {
					return err
				}
			}
		}
	}

	r1r, ok1 := r1.(RecvableRes)
	r2r, ok2 := r2.(RecvableRes)
	if ok1 != ok2 {
		return fmt.Errorf("recvable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		v1 := r1r.Recv()
		v2 := r2r.Recv()

		// XXX: Our Send/Recv in the lib/main.go doesn't seem to be
		// pulling this in, so this always compares differently. We can
		// comment it out for now, since it's not too consequential.
		// XXX: Find out what the issue is and fix it for here and send.
		// XXX: The below errors are commented out until this is fixed.
		if (v1 == nil) != (v2 == nil) { // xor
			//return fmt.Errorf("recv params differ")
		}
		if v1 != nil && v2 != nil {
			if len(v1) != len(v2) {
				//return fmt.Errorf("recv param lengths differ")
			}
			for key, send1 := range v1 { // map[string]*engine.Send
				send2, exists := v2[key]
				if !exists {
					//return fmt.Errorf("recv param key %s doesn't exist", key)
				}
				if (send1 == nil) != (send2 == nil) { // xor
					//return fmt.Errorf("recv param key %s send differs", key)
				}
				if send1 != nil && send2 != nil && send1.Key != send2.Key {
					//return fmt.Errorf("recv param key %s send key differs (%v != %v)", key, send1.Key, send2.Key)
				}
			}
		}
	}

	r1s, ok1 := r1.(SendableRes)
	r2s, ok2 := r2.(SendableRes)
	if ok1 != ok2 {
		return fmt.Errorf("sendable differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		s1 := r1s.Sent()
		s2 := r2s.Sent()

		// XXX: Our Send/Recv in the lib/main.go doesn't seem to be
		// pulling this in, so this always compares differently. We can
		// comment it out for now, since it's not too consequential.
		// XXX: Find out what the issue is and fix it for here and recv.
		// XXX: The below errors are commented out until this is fixed.
		if (s1 == nil) != (s2 == nil) { // xor
			//return fmt.Errorf("send params differ")
		}
		if s1 != nil && s2 != nil {
			// TODO: reflect.DeepEqual?
			//return fmt.Errorf("send params exist")
		}
	}

	// compare meta params for resources with reversible traits
	r1v, ok1 := r1.(ReversibleRes)
	r2v, ok2 := r2.(ReversibleRes)
	if ok1 != ok2 {
		return fmt.Errorf("reversible differs") // they must be different (optional)
	}
	if ok1 && ok2 {
		if r1v.ReversibleMeta().Cmp(r2v.ReversibleMeta()) != nil {
			return fmt.Errorf("reversible differs")
		}
	}

	return nil
}

// VertexCmpFn returns if two vertices are equivalent. It errors if they can't
// be compared because one is not a vertex. This returns true if equal.
// TODO: shouldn't the first argument be an `error` instead?
func VertexCmpFn(v1, v2 pgraph.Vertex) (bool, error) {
	r1, ok := v1.(Res)
	if !ok {
		return false, fmt.Errorf("v1 is not a Res")
	}
	r2, ok := v2.(Res)
	if !ok {
		return false, fmt.Errorf("v2 is not a Res")
	}

	if err := ResCmp(r1, r2); err != nil {
		//fmt.Printf("bad Cmp: %p %+v <> %p %+v err: %+v\n", r1, r1, r2, r2, err)
		return false, nil
	}
	//fmt.Printf("ok Cmp: %p %+v <> %p %+v\n", r1, r1, r2, r2)

	return true, nil
}

// EdgeCmpFn returns if two edges are equivalent. It errors if they can't be
// compared because one is not an edge. This returns true if equal.
// TODO: shouldn't the first argument be an `error` instead?
func EdgeCmpFn(e1, e2 pgraph.Edge) (bool, error) {
	edge1, ok := e1.(*Edge)
	if !ok {
		return false, fmt.Errorf("e1 is not an Edge")
	}
	edge2, ok := e2.(*Edge)
	if !ok {
		return false, fmt.Errorf("e2 is not an Edge")
	}
	return edge1.Cmp(edge2) == nil, nil
}

// ResGraphMapper compares two graphs, and gives us a mapping from new to old
// based on the resource kind and name only. This allows us to know which
// previous resource might have data to pass on to the new version in the next
// generation.
// FIXME: Optimize this for performance since it runs a lot...
func ResGraphMapper(oldGraph, newGraph *pgraph.Graph) (map[RecvableRes]RecvableRes, error) {
	mapper := make(map[RecvableRes]RecvableRes) // new -> old based on name and kind only?
	cmp := func(r1, r2 Res) error {
		if r1.Kind() != r2.Kind() {
			return fmt.Errorf("kind differs")
		}
		if r1.Name() != r2.Name() {
			return fmt.Errorf("name differs")
		}
		return nil
	}

	// XXX: run this as a topological sort or reverse topological sort?
	for v := range newGraph.Adjacency() { // loop through the vertices (resources)
		r, ok := v.(RecvableRes)
		if !ok {
			continue // skip
		}
		fn := func(vv pgraph.Vertex) (bool, error) {
			rr, ok := vv.(Res)
			if !ok {
				return false, fmt.Errorf("not a Res")
			}

			if err := cmp(rr, r); err != nil {
				return false, nil
			}
			return true, nil
		}
		vertex, err := oldGraph.VertexMatchFn(fn)
		if err != nil {
			return nil, errwrap.Wrapf(err, "VertexMatchFn failed")
		}
		if vertex == nil {
			continue // skip (error?)
		}
		res, ok := vertex.(RecvableRes)
		if !ok {
			continue // skip (error?)
		}

		mapper[r] = res
	}

	return mapper, nil
}

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

package engine

import (
	"fmt"

	"github.com/purpleidea/mgmt/pgraph"
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

		if (v1 == nil) != (v2 == nil) { // xor
			return fmt.Errorf("recv params differ")
		}
		if v1 != nil && v2 != nil {
			// TODO: until we hit this code path, don't allow
			// comparing anything that has this set to non-zero
			if len(v1) != 0 || len(v2) != 0 {
				return fmt.Errorf("recv params exist")
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

		if (s1 == nil) != (s2 == nil) { // xor
			return fmt.Errorf("send params differ")
		}
		if s1 != nil && s2 != nil {
			// TODO: until we hit this code path, don't allow
			// adapting anything that has this set to non-nil
			return fmt.Errorf("send params exist")
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

		if (v1 == nil) != (v2 == nil) { // xor
			return fmt.Errorf("recv params differ")
		}
		if v1 != nil && v2 != nil {
			// TODO: until we hit this code path, don't allow
			// adapting anything that has this set to non-zero
			if len(v1) != 0 || len(v2) != 0 {
				return fmt.Errorf("recv params exist")
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

		if (s1 == nil) != (s2 == nil) { // xor
			return fmt.Errorf("send params differ")
		}
		if s1 != nil && s2 != nil {
			// TODO: until we hit this code path, don't allow
			// adapting anything that has this set to non-nil
			return fmt.Errorf("send params exist")
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

	if ResCmp(r1, r2) != nil {
		return false, nil
	}

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

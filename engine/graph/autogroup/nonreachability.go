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

package autogroup

import (
	"github.com/purpleidea/mgmt/pgraph"

	errwrap "github.com/pkg/errors"
)

// NonReachabilityGrouper is the most straight-forward algorithm for grouping.
// TODO: this algorithm may not be correct in all cases. replace if needed!
type NonReachabilityGrouper struct {
	baseGrouper // "inherit" what we want, and reimplement the rest
}

// Name returns the name for the grouper algorithm.
func (ag *NonReachabilityGrouper) Name() string {
	return "NonReachabilityGrouper"
}

// VertexNext iteratively finds vertex pairs with simple graph reachability...
// This algorithm relies on the observation that if there's a path from a to b,
// then they *can't* be merged (b/c of the existing dependency) so therefore we
// merge anything that *doesn't* satisfy this condition or that of the reverse!
func (ag *NonReachabilityGrouper) VertexNext() (v1, v2 pgraph.Vertex, err error) {
	for {
		v1, v2, err = ag.baseGrouper.VertexNext() // get all iterable pairs
		if err != nil {
			return nil, nil, errwrap.Wrapf(err, "error running autoGroup(vertexNext)")
		}

		// ignore self cmp early (perf optimization)
		if v1 != v2 && v1 != nil && v2 != nil {
			// if NOT reachable, they're viable...
			out1, e1 := ag.graph.Reachability(v1, v2)
			if e1 != nil {
				return nil, nil, e1
			}
			out2, e2 := ag.graph.Reachability(v2, v1)
			if e2 != nil {
				return nil, nil, e2
			}
			if len(out1) == 0 && len(out2) == 0 {
				return // return v1 and v2, they're viable
			}
		}

		// if we got here, it means we're skipping over this candidate!
		if ok, err := ag.baseGrouper.VertexTest(false); err != nil {
			return nil, nil, errwrap.Wrapf(err, "error running autoGroup(vertexTest)")
		} else if !ok {
			return nil, nil, nil // done!
		}

		// the vertexTest passed, so loop and try with a new pair...
	}
}

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

package autogroup

import (
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// AutoGroup is the mechanical auto group "runner" that runs the interface spec.
// TODO: this algorithm may not be correct in all cases. replace if needed!
func AutoGroup(ag engine.AutoGrouper, g *pgraph.Graph, debug bool, logf func(format string, v ...interface{})) error {
	logf("algorithm: %s...", ag.Name())
	if err := ag.Init(g); err != nil {
		return errwrap.Wrapf(err, "error running autoGroup(init)")
	}

	for {
		var v, w pgraph.Vertex
		v, w, err := ag.VertexNext() // get pair to compare
		if err != nil {
			return errwrap.Wrapf(err, "error running autoGroup(vertexNext)")
		}
		merged := false
		// save names since they change during the runs
		vStr := fmt.Sprintf("%v", v) // valid even if it is nil
		wStr := fmt.Sprintf("%v", w)

		if err := ag.VertexCmp(v, w); err != nil { // cmp ?
			if debug {
				logf("!GroupCmp for: %s into: %s", wStr, vStr)
				logf("!GroupCmp err: %+v", err)
			}

			// remove grouped vertex and merge edges (res is safe)
		} else if err := VertexMerge(g, v, w, ag.VertexMerge, ag.EdgeMerge); err != nil { // merge...
			logf("!VertexMerge for: %s into: %s", wStr, vStr)
			if debug {
				logf("!VertexMerge err: %+v", err)
			}

		} else { // success!
			logf("%s into %s", wStr, vStr)
			merged = true // woo
		}

		// did these get used?
		if ok, err := ag.VertexTest(merged); err != nil {
			return errwrap.Wrapf(err, "error running autoGroup(vertexTest)")
		} else if !ok {
			break // done!
		}
	}

	// It would be great to ensure we didn't add any graph cycles here, but
	// instead of checking now, we'll move the check into the main loop.

	return nil
}

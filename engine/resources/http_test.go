// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package resources

import (
	"strings"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

// TestHTTPFileResAutoEdges tests the automatic edge generation for HTTP file
// resources to ensure they correctly create edges to file resources.
func TestHTTPFileResAutoEdges(t *testing.T) {
	// Create a new HTTP file resource with a path.
	obj := &HTTPFileRes{
		Path: "/tmp/test-file.txt",
	}

	// Get the auto edges.
	autoEdge, err := obj.AutoEdges()
	if err != nil {
		t.Errorf("unexpected error from AutoEdges: %v", err)
		return
	}

	// Get the first edge.
	edges := autoEdge.Next()
	if len(edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(edges))
		return
	}

	// Examine the edge.
	uid := edges[0]

	// Check that it's a HTTPFileUID.
	httpUID, ok := uid.(*HTTPFileUID)
	if !ok {
		t.Errorf("expected *HTTPFileUID, got %T", uid)
		return
	}

	// Check the kind using String() method.
	uidStr := uid.String()
	if !strings.Contains(uidStr, "file") {
		t.Errorf("expected uid String() to contain 'file', got: %s", uidStr)
	}

	// Check the path.
	if httpUID.path != "/tmp/test-file.txt" {
		t.Errorf("expected uid path to be '/tmp/test-file.txt', got: %s", httpUID.path)
	}

	// Test IFF matching by creating a mock file UID.
	mockUID := &FileUID{
		BaseUID: engine.BaseUID{
			Name: "/tmp/test-file.txt",
			Kind: "file",
		},
		path: "/tmp/test-file.txt",
	}

	if !httpUID.IFF(mockUID) {
		t.Errorf("iff should return true for matching paths")
	}

	// Test with Data field instead of Path (should have no edges).
	objWithData := &HTTPFileRes{
		Data: "some content",
	}

	autoEdgeData, err := objWithData.AutoEdges()
	if err != nil {
		t.Errorf("unexpected error from AutoEdges for Data case: %v", err)
		return
	}

	// Should have no edges.
	edgesData := autoEdgeData.Next()
	if edgesData != nil && len(edgesData) > 0 {
		t.Errorf("expected no edges for Data case, got %v", edgesData)
	}
}

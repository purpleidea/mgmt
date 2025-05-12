package resources

import (
	"strings"
	"testing"
)

// TestHTTPFileResAutoEdges tests the automatic edge generation
// for HTTP file resources to ensure they correctly create edges
// to file resources.
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
	mockUID := &HTTPFileUID{
		path: "/tmp/test-file.txt",
	}

	if !httpUID.IFF(mockUID) {
		t.Errorf("IFF should return true for matching paths")
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


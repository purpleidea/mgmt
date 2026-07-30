package strutil

import "testing"

func TestConcat(t *testing.T) {
	exp := "a b"
	got := Concat("a", " ", "b")
	if got != exp {
		t.Fatalf("expected %s, got %s", exp, got)
	}
}

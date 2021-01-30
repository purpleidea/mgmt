package types

import (
	"reflect"
	"testing"
)

func TestStructTagToFieldName0(t *testing.T) {
	type foo struct {
		A string `lang:"aaa"`
		B bool   `lang:"bbb"`
		C int64  `lang:"ccc"`
	}
	f := &foo{ // a ptr!
		A: "hello",
		B: true,
		C: 13,
	}
	m, err := StructTagToFieldName(f) // (map[string]string, error)
	if err != nil {
		t.Errorf("got error: %+v", err)
		return
	}
	t.Logf("got output: %+v", m)
	expected := map[string]string{
		"aaa": "A",
		"bbb": "B",
		"ccc": "C",
	}
	if !reflect.DeepEqual(m, expected) {
		t.Errorf("unexpected result")
		return
	}
}

func TestStructTagToFieldName1(t *testing.T) {
	type foo struct {
		A string `lang:"aaa"`
		B bool   `lang:"bbb"`
		C int64  `lang:"ccc"`
	}
	f := foo{ // not a ptr!
		A: "hello",
		B: true,
		C: 13,
	}
	m, err := StructTagToFieldName(f) // (map[string]string, error)
	if err == nil {
		t.Errorf("expected error, got nil")
		//return
	}
	t.Logf("got output: %+v", m)
	t.Logf("got error: %+v", err)
}

func TestStructTagToFieldName2(t *testing.T) {
	m, err := StructTagToFieldName(nil) // (map[string]string, error)
	if err == nil {
		t.Errorf("expected error, got nil")
		//return
	}
	t.Logf("got output: %+v", m)
	t.Logf("got error: %+v", err)
}

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

// +build !root

package resources

import (
	"reflect"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

func TestCmp1(t *testing.T) {
	r1, err := engine.NewResource("noop")
	if err != nil {
		t.Errorf("could not create resource: %+v", err)
	}
	r2, err := engine.NewResource("noop")
	if err != nil {
		t.Errorf("could not create resource: %+v", err)
	}
	r3, err := engine.NewResource("file")
	if err != nil {
		t.Errorf("could not create resource: %+v", err)
	}

	if err := r1.Cmp(r2); err != nil {
		t.Errorf("the two resources do not match: %+v", err)
	}
	if err := r2.Cmp(r1); err != nil {
		t.Errorf("the two resources do not match: %+v", err)
	}

	if r1.Cmp(r3) == nil {
		t.Errorf("the two resources should not match")
	}
	if r3.Cmp(r1) == nil {
		t.Errorf("the two resources should not match")
	}
}

func TestSort0(t *testing.T) {
	rs := []engine.Res{}
	s := engine.Sort(rs)

	if !reflect.DeepEqual(s, []engine.Res{}) {
		t.Errorf("sort failed!")
		if s == nil {
			t.Logf("output is nil!")
		} else {
			str := "Got:"
			for _, r := range s {
				str += " " + r.String()
			}
			t.Errorf(str)
		}
	}
}

func TestSort1(t *testing.T) {
	r1, _ := engine.NewNamedResource("noop", "noop1")
	r2, _ := engine.NewNamedResource("noop", "noop2")
	r3, _ := engine.NewNamedResource("noop", "noop3")
	r4, _ := engine.NewNamedResource("noop", "noop4")
	r5, _ := engine.NewNamedResource("noop", "noop5")
	r6, _ := engine.NewNamedResource("noop", "noop6")

	rs := []engine.Res{r3, r2, r6, r1, r5, r4}
	s := engine.Sort(rs)

	if !reflect.DeepEqual(s, []engine.Res{r1, r2, r3, r4, r5, r6}) {
		t.Errorf("sort failed!")
		str := "Got:"
		for _, r := range s {
			str += " " + r.String()
		}
		t.Errorf(str)
	}

	if !reflect.DeepEqual(rs, []engine.Res{r3, r2, r6, r1, r5, r4}) {
		t.Errorf("sort modified input!")
		str := "Got:"
		for _, r := range rs {
			str += " " + r.String()
		}
		t.Errorf(str)
	}
}

// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

//go:build !root

package facts

import (
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/pgraph"
)

func TestFuncGraph0(t *testing.T) {
	t.Logf("Hello!")
	g, _ := pgraph.NewGraph("empty") // empty graph

	obj := &funcs.Engine{
		Graph: g,
	}

	t.Logf("Init...")
	if err := obj.Init(); err != nil {
		t.Errorf("could not init: %+v", err)
		return
	}

	t.Logf("Validate...")
	if err := obj.Validate(); err != nil {
		t.Errorf("could not validate: %+v", err)
		return
	}

	t.Logf("Run...")
	if err := obj.Run(); err != nil {
		t.Errorf("could not run: %+v", err)
		return
	}

	// wait for some activity
	t.Logf("Stream...")
	stream := obj.Stream()
	t.Logf("Loop...")
	br := time.After(time.Duration(5) * time.Second)
Loop:
	for {
		select {
		case err, ok := <-stream:
			if !ok {
				t.Logf("Stream break...")
				break Loop
			}
			if err != nil {
				t.Logf("Error: %+v", err)
				continue
			}

		case <-br:
			t.Logf("Break...")
			t.Errorf("empty graph should have closed stream")
			break Loop
		}
	}

	t.Logf("Closing...")
	if err := obj.Close(); err != nil {
		t.Errorf("could not close: %+v", err)
		return
	}
}

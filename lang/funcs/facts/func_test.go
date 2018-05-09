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

// +build !root

package facts

import (
	"log"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/pgraph"
)

const Debug = false // switch on for more interactive log messages when testing!

// logf switches messages to use realtime logging when debugging tests, and the
// quiet logging which is not shown until test failures, when debug mode is off.
func logf(t *testing.T, format string, args ...interface{}) {
	if Debug {
		log.Printf(format, args...)
	} else {
		t.Logf(format, args...)
	}
}

func TestFuncGraph0(t *testing.T) {
	logf(t, "Hello!")
	g, _ := pgraph.NewGraph("empty") // empty graph

	obj := &funcs.Engine{
		Graph: g,
	}

	logf(t, "Init...")
	if err := obj.Init(); err != nil {
		t.Errorf("could not init: %+v", err)
		return
	}

	logf(t, "Validate...")
	if err := obj.Validate(); err != nil {
		t.Errorf("could not validate: %+v", err)
		return
	}

	logf(t, "Run...")
	if err := obj.Run(); err != nil {
		t.Errorf("could not run: %+v", err)
		return
	}

	// wait for some activity
	logf(t, "Stream...")
	stream := obj.Stream()
	logf(t, "Loop...")
	br := time.After(time.Duration(5) * time.Second)
Loop:
	for {
		select {
		case err, ok := <-stream:
			if !ok {
				logf(t, "Stream break...")
				break Loop
			}
			if err != nil {
				logf(t, "Error: %+v", err)
				continue
			}

		case <-br:
			logf(t, "Break...")
			t.Errorf("empty graph should have closed stream")
			break Loop
		}
	}

	logf(t, "Closing...")
	if err := obj.Close(); err != nil {
		t.Errorf("could not close: %+v", err)
		return
	}
}

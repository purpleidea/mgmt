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

// Package gapi defines the interface that graph API generators must meet.
package gapi

import (
	"encoding/gob"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"

	"github.com/urfave/cli"
)

// RegisteredGAPIs is a global map of all possible GAPIs which can be used. You
// should never touch this map directly. Use methods like Register instead.
var RegisteredGAPIs = make(map[string]func() GAPI) // must initialize this map

// Register takes a GAPI and its name and makes it available for use. There is
// no matching Unregister function.
func Register(name string, fn func() GAPI) {
	if _, ok := RegisteredGAPIs[name]; ok {
		panic(fmt.Sprintf("a GAPI named %s is already registered", name))
	}
	gob.Register(fn())
	RegisteredGAPIs[name] = fn
}

// Data is the set of input values passed into the GAPI structs via Init.
type Data struct {
	Program       string // name of the originating program
	Hostname      string // uuid for the host, required for GAPI
	World         engine.World
	Noop          bool
	NoConfigWatch bool
	NoStreamWatch bool
	Prefix        string
	Debug         bool
	Logf          func(format string, v ...interface{})
	// NOTE: we can add more fields here if needed by GAPI endpoints
}

// Next describes the particular response the GAPI implementer wishes to emit.
type Next struct {
	// FIXME: the Fast pause parameter should eventually get replaced with a
	// "SwitchMethod" parameter or similar that instead lets the implementer
	// choose between fast pause, slow pause, and interrupt. Interrupt could
	// be a future extension to the Resource API that lets an Interrupt() be
	// called if we want to exit immediately from the CheckApply part of the
	// resource for some reason. For now we'll keep this simple with a bool.
	Fast bool  // run a fast pause to switch?
	Exit bool  // should we cause the program to exit? (specify err or not)
	Err  error // if something goes wrong (use with or without exit!)
}

// GAPI is a Graph API that represents incoming graphs and change streams.
type GAPI interface {
	Cli(c *cli.Context, fs engine.Fs) (*Deploy, error)
	CliFlags() []cli.Flag

	Init(Data) error               // initializes the GAPI and passes in useful data
	Graph() (*pgraph.Graph, error) // returns the most recent pgraph
	Next() chan Next               // returns a stream of switch events
	Close() error                  // shutdown the GAPI
}

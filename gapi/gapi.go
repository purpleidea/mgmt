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

// Package gapi defines the interface that graph API generators must meet.
package gapi

import (
	"encoding/gob"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/pgraph"

	"github.com/urfave/cli"
)

const (
	// CommandRun is the identifier for the "run" command. It is distinct
	// from the other commands, because it can run with any front-end.
	CommandRun = "run"

	// CommandDeploy is the identifier for the "deploy" command.
	CommandDeploy = "deploy"

	// CommandGet is the identifier for the "get" (download) command.
	CommandGet = "get"
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

// CliInfo is the set of input values passed into the Cli method so that the
// GAPI can decide if it wants to activate, and if it does, the initial handles
// it needs to use to do so.
type CliInfo struct {
	// CliContext is the struct that is used to transfer in user input.
	CliContext *cli.Context
	// Fs is the filesystem the Cli method should copy data into. It usually
	// copies *from* the local filesystem using standard io functionality.
	Fs    engine.Fs
	Debug bool
	Logf  func(format string, v ...interface{})
}

// Data is the set of input values passed into the GAPI structs via Init.
type Data struct {
	Program       string // name of the originating program
	Hostname      string // uuid for the host, required for GAPI
	World         engine.World
	Noop          bool
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

// GAPI is a Graph API that represents incoming graphs and change streams. It is
// the frontend interface that needs to be implemented to use the engine.
type GAPI interface {
	// CliFlags is passed a Command constant specifying which command it is
	// requesting the flags for. If an invalid or unsupported command is
	// passed in, simply return an empty list. Similarly, it is not required
	// to ever return any flags, and the GAPI may always return an empty
	// list.
	CliFlags(string) []cli.Flag

	// Cli is run on each GAPI to give it a chance to decide if it wants to
	// activate, and if it does, then it will return a deploy struct. During
	// this time, it uses the CliInfo struct as useful information to decide
	// what to do.
	Cli(*CliInfo) (*Deploy, error)

	// Init initializes the GAPI and passes in some useful data.
	Init(*Data) error

	// Graph returns the most recent pgraph. This is called by the engine on
	// every event from Next().
	Graph() (*pgraph.Graph, error)

	// Next returns a stream of switch events. The engine will run Graph()
	// to build a new graph after every Next event.
	Next() chan Next

	// Close shuts down the GAPI. It asks the GAPI to close, and must cause
	// Next() to unblock even if is currently blocked and waiting to send a
	// new event.
	Close() error
}

// GetInfo is the set of input values passed into the Get method for it to run.
type GetInfo struct {
	// CliContext is the struct that is used to transfer in user input.
	CliContext *cli.Context

	Noop   bool
	Sema   int
	Update bool

	Debug bool
	Logf  func(format string, v ...interface{})
}

// GettableGAPI represents additional methods that need to be implemented in
// this GAPI so that it can be used with the `get` Command. The methods in this
// interface are called independently from the rest of the GAPI interface, and
// you must not rely on shared state from those methods. Logically, this should
// probably be named "Getable", however the correct modern word is "Gettable".
type GettableGAPI interface {
	GAPI // the base interface must be implemented

	// Get runs the get/download method.
	Get(*GetInfo) error
}

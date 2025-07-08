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

// Package gapi defines the interface that graph API generators must meet.
package gapi

import (
	"context"
	"encoding/gob"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/pgraph"
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

// Names returns a list of the names of all registered GAPIs.
func Names() []string {
	names := []string{}
	for name := range RegisteredGAPIs {
		names = append(names, name)
	}
	return names
}

// Flags is some common data that comes from a higher-level command, and is used
// by a subcommand. By type circularity, the subcommands can't easily access the
// data in the parent command struct, so instead, the data that the parent wants
// to pass down, it wraps up in a struct (for API convenience) and sends it out.
type Flags struct {
	Hostname *string

	Noop bool
	Sema int

	NoAutoEdges bool
}

// Info is the set of input values passed into the Cli method so that the GAPI
// can decide if it wants to activate, and if it does, the initial handles it
// needs to use to do so.
type Info struct {
	// Args are the CLI args that are populated after parsing the args list.
	// They need to be converted to the struct you are expecting to read it.
	Args interface{}

	// Flags are the common data which is passed down into the sub command.
	Flags *Flags

	// Fs is the filesystem the Cli method should copy data into. It usually
	// copies *from* the local filesystem using standard io functionality.
	Fs engine.Fs

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Data is the set of input values passed into the GAPI structs via Init.
type Data struct {
	Program       string // name of the originating program
	Version       string // version of the originating program
	Hostname      string // uuid for the host, required for GAPI
	Local         *local.API
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
	// Graph returns the current resource graph.
	Graph *pgraph.Graph

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

// InfoResult is some data that a GAPI can return on request.
type InfoResult struct {
	// URI is the FS URI that we pass around everywhere.
	// TODO: can this be deprecated?
	URI string
}

// GAPI is a Graph API that represents incoming graphs and change streams. It is
// the frontend interface that needs to be implemented to use the engine.
type GAPI interface {
	// Cli is run on each GAPI to give it a chance to decide if it wants to
	// activate, and if it does, then it will return a deploy struct. During
	// this time, it uses the Info struct as useful information to decide
	// what to do.
	Cli(*Info) (*Deploy, error)

	// Init initializes the GAPI and passes in some useful data.
	Init(*Data) error

	// Info returns some data about the GAPI implementation.
	Info() *InfoResult

	// Next returns a stream of events. Each next event contains a resource
	// graph.
	Next(ctx context.Context) chan Next

	// Err will contain the last error when Next shuts down. It waits for
	// all the running processes to exit before it returns.
	Err() error
}

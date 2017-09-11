// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"
)

// Data is the set of input values passed into the GAPI structs via Init.
type Data struct {
	Hostname      string // uuid for the host, required for GAPI
	World         resources.World
	Noop          bool
	NoConfigWatch bool
	NoStreamWatch bool
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
	Init(Data) error               // initializes the GAPI and passes in useful data
	Graph() (*pgraph.Graph, error) // returns the most recent pgraph
	Next() chan Next               // returns a stream of switch events
	Close() error                  // shutdown the GAPI
}

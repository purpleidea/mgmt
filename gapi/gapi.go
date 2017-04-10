// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
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

// GAPI is a Graph API that represents incoming graphs and change streams.
type GAPI interface {
	Init(Data) error               // initializes the GAPI and passes in useful data
	Graph() (*pgraph.Graph, error) // returns the most recent pgraph
	Next() chan error              // returns a stream of switch events
	Close() error                  // shutdown the GAPI
}

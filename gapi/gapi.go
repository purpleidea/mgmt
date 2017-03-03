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

// World is an interface to the rest of the different graph state. It allows
// the GAPI to store state and exchange information throughout the cluster. It
// is the interface each machine uses to communicate with the rest of the world.
type World interface { // TODO: is there a better name for this interface?
	ResExport([]resources.Res) error
	// FIXME: should this method take a "filter" data struct instead of many args?
	ResCollect(hostnameFilter, kindFilter []string) ([]resources.Res, error)

	StrWatch(namespace string) chan error
	StrGet(namespace string) (map[string]string, error)
	StrSet(namespace, value string) error
	StrDel(namespace string) error
}

// Data is the set of input values passed into the GAPI structs via Init.
type Data struct {
	Hostname string // uuid for the host, required for GAPI
	World    World
	Noop     bool
	NoWatch  bool
	// NOTE: we can add more fields here if needed by GAPI endpoints
}

// GAPI is a Graph API that represents incoming graphs and change streams.
type GAPI interface {
	Init(Data) error               // initializes the GAPI and passes in useful data
	Graph() (*pgraph.Graph, error) // returns the most recent pgraph
	Next() chan error              // returns a stream of switch events
	Close() error                  // shutdown the GAPI
}

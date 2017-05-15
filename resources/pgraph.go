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

package resources

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/pgraph"
)

//go:generate stringer -type=graphState -output=graphstate_stringer.go
type graphState uint

const (
	graphStateNil graphState = iota
	graphStateStarting
	graphStateStarted
	graphStatePausing
	graphStatePaused
)

// getState returns the state of the graph. This state is used for optimizing
// certain algorithms by knowing what part of processing the graph is currently
// undergoing.
func getState(g *pgraph.Graph) graphState {
	//mutex := StateLockFromGraph(g)
	//mutex.Lock()
	//defer mutex.Unlock()
	if u, ok := g.Value("state"); ok {
		return util.Uint(u)
	}
	return graphStateNil
}

// setState sets the graph state and returns the previous state.
func setState(g *pgraph.Graph, state graphState) graphState {
	mutex := StateLockFromGraph(g)
	mutex.Lock()
	defer mutex.Unlock()
	prev := getState(g)
	g.SetValue("state", state)
	return prev
}

// StateLockFromGraph returns a pointer to the state lock stored with the graph,
// otherwise it panics. If one does not exist, it will create it.
func StateLockFromGraph(g *pgraph.Graph) *sync.Mutex {
	x, exists := g.Value("mutex")
	if !exists {
		g.SetValue("mutex", &sync.Mutex{})
		x, _ = g.Value("mutex")
	}

	m, ok := x.(*sync.Mutex)
	if !ok {
		panic("not a *sync.Mutex")
	}
	return m
}

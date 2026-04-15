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

package scheduler

import (
	"context"
	"fmt"
)

func init() {
	Register(func() Strategy { return &nilStrategy{} }) // must register
}

// registeredStrategies is a global map of all possible strategy implementations
// which can be used. You should never touch this map directly. Use methods like
// Register instead.
var registeredStrategies = make(map[string]func() Strategy) // must initialize

// Strategy represents the methods a scheduler strategy must implement.
type Strategy interface {
	Kind() string

	// Schedule is called with a map of hostnames that are available to
	// schedule. The values in the map correspond to available data that may
	// be used in determining which hosts to prefer. The opts are general
	// scheduling options which are not specific to any specific host. The
	// result is the chosen set of hostnames and must not contain duplicates
	// or any value not present as a key in the incoming map.
	Schedule(ctx context.Context, hostnames map[string]string, params *Params) ([]string, error)
}

// Params is a struct of fields which may be used by the Schedule strategies.
type Params struct {
	// Options are the common, generic scheduling options which are
	// available to all scheduling strategies.
	Options *Options

	// Last is a function which may be called to determine the last
	// scheduling decision. This call isn't free, so it should only be used
	// if necessary. If the Strategy can cache this result in its struct for
	// subsequent calls during a period where it has not switched to running
	// on a new machine, then that is recommended. A scheduling strategy can
	// determine if it needs to run this again if it sees a cache of zero
	// length. In this scenario, not a lot of scheduling has been done even
	// if that was the last scenario, so it's fine to run Last to get this
	// data. This is important because the scheduling strategy can migrate
	// and run from different machines over time. The ctx is used to cancel
	// this operation quickly if the parent ctx of Schedule is cancelled.
	Last func(context.Context) ([]string, error)

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Register takes a func containing a strategy implementation and stores it for
// future use. It is called at program startup in the init() method of the file
// where the strategy is defined. There is no matching Unregister function.
func Register(fn func() Strategy) {
	f := fn()
	kind := f.Kind()
	if _, ok := registeredStrategies[kind]; ok {
		panic(fmt.Sprintf("a strategy kind of %s is already registered", kind))
	}
	//gob.Register(f)
	registeredStrategies[kind] = fn
}

// Lookup returns a new strategy from the previously registered ones.
func Lookup(kind string) (Strategy, error) {
	f, exists := registeredStrategies[kind]
	if !exists {
		return nil, fmt.Errorf("undefined strategy: %s", kind)
	}
	return f(), nil
}

type nilStrategy struct{}

// Kind returns a kind for the strategy.
func (obj *nilStrategy) Kind() string { return "" } // the "nil" kind

// Schedule returns an error for any scheduling request for this nil strategy.
func (obj *nilStrategy) Schedule(context.Context, map[string]string, *Params) ([]string, error) {
	return nil, fmt.Errorf("cannot schedule with nil scheduler")
}

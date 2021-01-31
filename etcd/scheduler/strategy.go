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

package scheduler

import (
	"fmt"
)

// registeredStrategies is a global map of all possible strategy implementations
// which can be used. You should never touch this map directly. Use methods like
// Register instead.
var registeredStrategies = make(map[string]func() Strategy) // must initialize

// Strategy represents the methods a scheduler strategy must implement.
type Strategy interface {
	Schedule(hostnames map[string]string, opts *schedulerOptions) ([]string, error)
}

// Register takes a func and its name and makes it available for use. It is
// commonly called in the init() method of the func at program startup. There is
// no matching Unregister function.
func Register(name string, fn func() Strategy) {
	if _, ok := registeredStrategies[name]; ok {
		panic(fmt.Sprintf("a strategy named %s is already registered", name))
	}
	//gob.Register(fn())
	registeredStrategies[name] = fn
}

type nilStrategy struct {
}

// Schedule returns an error for any scheduling request for this nil strategy.
func (obj *nilStrategy) Schedule(hostnames map[string]string, opts *schedulerOptions) ([]string, error) {
	return nil, fmt.Errorf("scheduler: cannot schedule with nil scheduler")
}

// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

// Option is a type that can be used to configure the scheduler.
type Option func(*schedulerOptions)

// schedulerOptions represents the different possible configurable options. Not
// all options necessarily work for each scheduler strategy algorithm.
type schedulerOptions struct {
	debug       bool
	logf        func(format string, v ...interface{})
	strategy    Strategy
	maxCount    int // TODO: should this be *int to know when it's set?
	reuseLease  bool
	sessionTTL  int // TODO: should this be *int to know when it's set?
	hostsFilter []string
	// TODO: add more options
}

// Debug specifies whether we should run in debug mode or not.
func Debug(debug bool) Option {
	return func(so *schedulerOptions) {
		so.debug = debug
	}
}

// Logf passes a logger function that we can use if so desired.
func Logf(logf func(format string, v ...interface{})) Option {
	return func(so *schedulerOptions) {
		so.logf = logf
	}
}

// StrategyKind sets the scheduler strategy used.
func StrategyKind(strategy string) Option {
	return func(so *schedulerOptions) {
		f, exists := registeredStrategies[strategy]
		if !exists {
			panic(fmt.Sprintf("scheduler: undefined strategy: %s", strategy))
		}
		so.strategy = f()
	}
}

// MaxCount is the maximum number of hosts that should get simultaneously
// scheduled.
func MaxCount(maxCount int) Option {
	return func(so *schedulerOptions) {
		if maxCount > 0 {
			so.maxCount = maxCount
		}
	}
}

// ReuseLease specifies whether we should try and re-use the lease between runs.
// Ordinarily it would get discarded with each new version (deploy) of the code.
func ReuseLease(reuseLease bool) Option {
	return func(so *schedulerOptions) {
		so.reuseLease = reuseLease
	}
}

// SessionTTL is the amount of time to delay before expiring a key on abrupt
// host disconnect of if ReuseLease is true.
func SessionTTL(sessionTTL int) Option {
	return func(so *schedulerOptions) {
		if sessionTTL > 0 {
			so.sessionTTL = sessionTTL
		}
	}
}

// HostsFilter specifies a manual list of hosts, to use as a subset of whatever
// was auto-discovered.
// XXX: think more about this idea...
func HostsFilter(hosts []string) Option {
	return func(so *schedulerOptions) {
		so.hostsFilter = hosts
	}
}

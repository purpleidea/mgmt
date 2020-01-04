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

package scheduler // TODO: i'd like this to be a separate package, but cycles!

import (
	"fmt"
	"sort"
)

func init() {
	Register("alpha", func() Strategy { return &alphaStrategy{} }) // must register the func and name
}

type alphaStrategy struct {
	// no state to store
}

// Schedule returns the first host out of a sorted group of available hostnames.
func (obj *alphaStrategy) Schedule(hostnames map[string]string, opts *schedulerOptions) ([]string, error) {
	if len(hostnames) <= 0 {
		return nil, fmt.Errorf("strategy: cannot schedule from zero hosts")
	}
	if opts.maxCount <= 0 {
		return nil, fmt.Errorf("strategy: cannot schedule with a max of zero")
	}

	sortedHosts := []string{}
	for key := range hostnames {
		sortedHosts = append(sortedHosts, key)
	}
	sort.Strings(sortedHosts)

	return []string{sortedHosts[0]}, nil // pick first host
}

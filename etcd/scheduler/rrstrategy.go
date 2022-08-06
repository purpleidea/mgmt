// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

	"github.com/purpleidea/mgmt/util"
)

func init() {
	Register("rr", func() Strategy { return &rrStrategy{} }) // must register the func and name
}

type rrStrategy struct {
	// some stored state
	hosts []string
}

// Schedule returns hosts in round robin style from the available hostnames.
func (obj *rrStrategy) Schedule(hostnames map[string]string, opts *schedulerOptions) ([]string, error) {
	if len(hostnames) <= 0 {
		return nil, fmt.Errorf("strategy: cannot schedule from zero hosts")
	}
	if opts.maxCount <= 0 {
		return nil, fmt.Errorf("strategy: cannot schedule with a max of zero")
	}

	// always get a deterministic list of current hosts first...
	sortedHosts := []string{}
	for key := range hostnames {
		sortedHosts = append(sortedHosts, key)
	}
	sort.Strings(sortedHosts)

	if obj.hosts == nil {
		obj.hosts = []string{} // initialize if needed
	}

	// add any new hosts we learned about, to the end of the list
	for _, x := range sortedHosts {
		if !util.StrInList(x, obj.hosts) {
			obj.hosts = append(obj.hosts, x)
		}
	}

	// remove any hosts we previouly knew about from the list
	for ix := len(obj.hosts) - 1; ix >= 0; ix-- {
		if !util.StrInList(obj.hosts[ix], sortedHosts) {
			// delete entry at this index
			obj.hosts = append(obj.hosts[:ix], obj.hosts[ix+1:]...)
		}
	}

	// get the maximum number of hosts to return
	max := len(obj.hosts)    // can't return more than we have
	if opts.maxCount < max { // found a smaller limit
		max = opts.maxCount
	}

	result := []string{}
	// now return the number of needed hosts from the list
	for i := 0; i < max; i++ {
		result = append(result, obj.hosts[i])
	}

	return result, nil
}

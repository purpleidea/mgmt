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
	"sort"
)

func init() {
	Register(func() Strategy { return &rrStrategy{} }) // must register
}

type rrStrategy struct {
	// hosts is a local cache of previously scheduled hosts.
	hosts []string
}

// Kind returns a kind for the strategy.
func (obj *rrStrategy) Kind() string { return "rr" }

// Schedule returns hosts in round robin style from the available hostnames.
func (obj *rrStrategy) Schedule(ctx context.Context, hostnames map[string]string, params *Params) ([]string, error) {
	// This is a naive rr scheduler, and when the host winning the Campaign
	// dies, the subsequent host that takes over and runs this, will not
	// have all the state and we'd churn and offer an unstable result! As a
	// result, we cache the previous state locally (to look it up quickly if
	// we haven't swapped scheduling hosts) and otherwise we look it up from
	// the Last() API which provides this data into the distributed system.

	if params == nil || params.Options == nil || params.Last == nil {
		// programming error
		return nil, fmt.Errorf("invalid params struct")
	}
	if len(hostnames) <= 0 {
		//return nil, fmt.Errorf("strategy: cannot schedule from zero hosts")
		return []string{}, nil // empty set
	}
	maxCount := 0
	if d := params.Options.MaxCount; d != nil {
		maxCount = *d
	}
	if maxCount <= 0 { // XXX: why not let it choose zero?
		//return nil, fmt.Errorf("strategy: cannot schedule with a max of zero")
		return []string{}, nil // empty set
	}

	// always get a deterministic list of current hosts first...
	sortedHosts := []string{}
	for key := range hostnames {
		sortedHosts = append(sortedHosts, key)
	}
	sort.Strings(sortedHosts)

	if obj.hosts == nil || len(obj.hosts) == 0 {
		//obj.hosts = []string{} // initialize if needed
		hosts, err := params.Last(ctx)
		if err != nil {
			return nil, err
		}
		obj.hosts = hosts
	}

	lookupCache := make(map[string]struct{}, len(obj.hosts))
	for _, x := range obj.hosts {
		lookupCache[x] = struct{}{}
	}

	// add any new hosts we learned about, to the end of the list
	for _, x := range sortedHosts {
		// without cache: !util.StrInList(x, obj.hosts)
		if _, exists := lookupCache[x]; !exists {
			obj.hosts = append(obj.hosts, x)
		}
	}

	lookupCache = make(map[string]struct{}, len(sortedHosts))
	for _, x := range sortedHosts {
		lookupCache[x] = struct{}{}
	}

	// remove any hosts we previously knew about from the list
	for ix := len(obj.hosts) - 1; ix >= 0; ix-- {
		// without cache: !util.StrInList(obj.hosts[ix], sortedHosts)
		if _, exists := lookupCache[obj.hosts[ix]]; !exists {
			// delete entry at this index
			obj.hosts = append(obj.hosts[:ix], obj.hosts[ix+1:]...)
		}
	}

	// get the maximum number of hosts to return
	max := min(maxCount, len(obj.hosts)) // can't return more than we have

	result := []string{}
	// now return the number of needed hosts from the list
	for i := 0; i < max; i++ {
		result = append(result, obj.hosts[i])
	}

	return result, nil
}

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
	Register(func() Strategy { return &alphaStrategy{} }) // must register
}

type alphaStrategy struct {
	// no state to store
}

// Kind returns a kind for the strategy.
func (obj *alphaStrategy) Kind() string { return "alpha" }

// Schedule returns the alphabetically earliest hosts out of a sorted group of
// available hostnames.
func (obj *alphaStrategy) Schedule(ctx context.Context, hostnames map[string]string, params *Params) ([]string, error) {
	// NOTE: this algorithm is intrinsically persistent, so it doesn't need
	// to look at the previous decision to make a stable subsequent choice.

	if params == nil || params.Options == nil {
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

	sortedHosts := []string{}
	for key := range hostnames {
		sortedHosts = append(sortedHosts, key)
	}
	sort.Strings(sortedHosts)

	// get the maximum number of hosts to return
	max := min(maxCount, len(sortedHosts)) // can't return more than we have

	result := []string{}
	// now return the number of needed hosts from the list
	for i := 0; i < max; i++ {
		result = append(result, sortedHosts[i])
	}

	return result, nil // pick first N hosts
}

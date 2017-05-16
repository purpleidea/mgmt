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
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/semaphore"

	multierr "github.com/hashicorp/go-multierror"
)

// SemaSep is the trailing separator to split the semaphore id from the size.
const SemaSep = ":"

// SemaLock acquires the list of semaphores in the graph.
func SemaLock(g *pgraph.Graph, semas []string) error {
	var reterr error
	sort.Strings(semas) // very important to avoid deadlock in the dag!
	slock := SemaLockFromGraph(g)
	smap := SemaMapFromGraph(g) // returns a map, which can be modified by ref

	for _, id := range semas {
		slock.Lock()         // semaphore creation lock
		sema, ok := smap[id] // lookup
		if !ok {
			size := SemaSize(id) // defaults to 1
			smap[id] = semaphore.NewSemaphore(size)
			sema = smap[id]
		}
		slock.Unlock()

		if err := sema.P(1); err != nil { // lock!
			reterr = multierr.Append(reterr, err) // list of errors
		}
	}
	return reterr
}

// SemaUnlock releases the list of semaphores in the graph.
func SemaUnlock(g *pgraph.Graph, semas []string) error {
	var reterr error
	sort.Strings(semas) // unlock in the same order to remove partial locks
	smap := SemaMapFromGraph(g)

	for _, id := range semas {
		sema, ok := smap[id] // lookup
		if !ok {
			// programming error!
			panic(fmt.Sprintf("graph: sema: %s does not exist", id))
		}

		if err := sema.V(1); err != nil { // unlock!
			reterr = multierr.Append(reterr, err) // list of errors
		}
	}
	return reterr
}

// SemaSize returns the size integer associated with the semaphore id. It
// defaults to 1 if not found.
func SemaSize(id string) int {
	size := 1 // default semaphore size
	// valid id's include "some_id", "hello:42" and ":13"
	if index := strings.LastIndex(id, SemaSep); index > -1 && (len(id)-index+len(SemaSep)) >= 1 {
		// NOTE: we only allow size > 0 here!
		if i, err := strconv.Atoi(id[index+len(SemaSep):]); err == nil && i > 0 {
			size = i
		}
	}
	return size
}

// SemaLockFromGraph returns a pointer to the semaphore lock stored with the
// graph, otherwise it panics. If one does not exist, it will create it.
func SemaLockFromGraph(g *pgraph.Graph) *sync.Mutex {
	x, exists := g.Value("slock")
	if !exists {
		g.SetValue("slock", &sync.Mutex{})
		x, _ = g.Value("slock")
	}

	slock, ok := x.(*sync.Mutex)
	if !ok {
		panic("not a *sync.Mutex")
	}
	return slock
}

// SemaMapFromGraph returns a pointer to the map of semaphores stored with the
// graph, otherwise it panics. If one does not exist, it will create it.
func SemaMapFromGraph(g *pgraph.Graph) map[string]*semaphore.Semaphore {
	x, exists := g.Value("semas")
	if !exists {
		semas := make(map[string]*semaphore.Semaphore)
		g.SetValue("semas", semas)
		x, _ = g.Value("semas")
	}

	semas, ok := x.(map[string]*semaphore.Semaphore)
	if !ok {
		panic("not a map[string]*semaphore.Semaphore")
	}
	return semas
}

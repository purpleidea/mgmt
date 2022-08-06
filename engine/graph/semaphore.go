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

package graph

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/semaphore"
)

// SemaSep is the trailing separator to split the semaphore id from the size.
const SemaSep = ":"

// semaLock acquires the list of semaphores in the graph.
func (obj *Engine) semaLock(semas []string) error {
	var reterr error
	sort.Strings(semas) // very important to avoid deadlock in the dag!

	for _, id := range semas {
		obj.slock.Lock()          // semaphore creation lock
		sema, ok := obj.semas[id] // lookup
		if !ok {
			size := SemaSize(id) // defaults to 1
			obj.semas[id] = semaphore.NewSemaphore(size)
			sema = obj.semas[id]
		}
		obj.slock.Unlock()

		err := sema.P(1)                     // lock!
		reterr = errwrap.Append(reterr, err) // list of errors
	}
	return reterr
}

// semaUnlock releases the list of semaphores in the graph.
func (obj *Engine) semaUnlock(semas []string) error {
	var reterr error
	sort.Strings(semas) // unlock in the same order to remove partial locks

	for _, id := range semas {
		sema, ok := obj.semas[id] // lookup
		if !ok {
			// programming error!
			panic(fmt.Sprintf("graph: sema: %s does not exist", id))
		}

		err := sema.V(1)                     // unlock!
		reterr = errwrap.Append(reterr, err) // list of errors
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

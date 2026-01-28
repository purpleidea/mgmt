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

package resources

import (
	"fmt"
	"sync"
)

func init() {
	bmcMapMutex = &sync.Mutex{}
	bmcMapState = make(map[string]*bmcState)
}

var (
	// bmcMapMutex is a mutex for blocking global bmc operations, and for
	// locks on the bmcStateMap map.
	bmcMapMutex *sync.Mutex

	// bmcStateMap is a map from unique bmc ID to corresponding bmc state.
	// TODO: add fields to the map value if needed...
	bmcMapState map[string]*bmcState
)

const (
	// BmcDriverSecureSuffix is the magic char we append to a driver name to
	// specify we want the SSL/TLS variant.
	BmcDriverSecureSuffix = "s"

	// BmcDriverRPC is the RPC driver.
	BmcDriverRPC = "rpc"

	// BmcDriverGofish is the gofish driver.
	BmcDriverGofish = "gofish"
)

func bmcStateReserve(name string) *bmcState {
	bmcMapMutex.Lock()
	defer bmcMapMutex.Unlock()

	state, exists := bmcMapState[name]
	if !exists {
		state = &bmcState{ // new!
			name:  name, // index "pointer" to the map
			mutex: &sync.Mutex{},
			count: 0,
		}
		bmcMapState[name] = state // store
	}
	state.count++
	return state
}

type bmcState struct {
	name  string // index in bmcMapState
	mutex *sync.Mutex
	count uint8
}

func (obj *bmcState) Release() {
	bmcMapMutex.Lock()
	defer bmcMapMutex.Unlock()

	state, exists := bmcMapState[obj.name]
	if !exists {
		// programming error
		panic(fmt.Sprintf("bmc state %s doesn't exist", obj.name))
	}
	state.count--
	if state.count < 0 {
		// programming error
		panic("bmc state count is negative")
	}
	if state.count == 0 {
		delete(bmcMapState, obj.name)
	}
}

func (obj *bmcState) Lock() {
	obj.mutex.Lock()
}

func (obj *bmcState) Unlock() {
	obj.mutex.Unlock()
}

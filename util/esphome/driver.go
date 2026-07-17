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

package esphome

import (
	"context"
)

// newDriverFunc builds the driver that new sessions use. It's a variable so
// that the tests can substitute a fake, and so that swapping the underlying
// wire library is a one-line change.
var newDriverFunc = newAPIClientDriver

// driver is the seam that hides the wire protocol library from the session
// logic. One driver represents one connection attempt: build it, connect it,
// use it, close it, and then throw it away. Implementations must be safe for
// concurrent use after connect returns.
type driver interface {
	// connect dials the device and performs the handshake. The ctx is the
	// lifetime of the connection: cancelling it must shut everything down.
	connect(ctx context.Context, info *ConnInfo) error

	// entities lists the entities that the device advertises. It must only
	// be called after connect succeeds.
	entities() ([]*EntityInfo, error)

	// subscribe asks the device to push state updates, which arrive on the
	// given callback, starting with a snapshot of every entity. It must
	// only be called after connect succeeds.
	subscribe(fn func(*EntityState)) error

	// subscribeLogs asks the device to push logger messages at or above the
	// requested level. It must only be called after connect succeeds.
	subscribeLogs(level string, fn func(*LogEntry)) error

	// done returns a channel which closes when the connection dies for any
	// reason.
	done() <-chan struct{}

	// close tears the connection down.
	close() error

	// setSwitch commands a switch entity by key.
	setSwitch(key uint32, on bool) error

	// setNumber commands a number entity by key.
	setNumber(key uint32, value float64) error

	// pressButton presses a button entity by key.
	pressButton(key uint32) error
}

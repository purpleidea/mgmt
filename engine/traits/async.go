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

package traits

// Async lets a resource opt in to engine-managed async CheckApply execution.
// This allows a graph swap pause to proceed while the CheckApply keeps running.
// The Meta:async metaparameter can override this when set. If the override
// setting is incompatible with the resource (for example, a resource which may
// *not* run in an async way) then this can be caught during the res Validate.
// The CheckApply must not use any special methods which depend on a Pause, such
// as the FilteredGraph API.
// TODO: If FilteredGraph turns out to be useful, make it safe with a mutex.
type Async struct {
	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// AsyncCheckApply tells the engine the async default for this resource. This
// can be overridden by Meta:async.
func (obj *Async) AsyncCheckApply() bool {
	return true
}

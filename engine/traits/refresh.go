// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package traits

// Refreshable functions as flag storage for resources to signal that they
// support receiving refresh notifications, and what that value is. These are
// commonly used to send information that some aspect of the state is invalid
// due to an unlinked change. The canonical example is a svc resource that needs
// reloading after a configuration file changes.
type Refreshable struct {
	refresh bool

	// Bug5819 works around issue https://github.com/golang/go/issues/5819
	Bug5819 interface{} // XXX: workaround
}

// Refresh returns the refresh notification state.
func (obj *Refreshable) Refresh() bool {
	return obj.refresh
}

// SetRefresh sets the refresh notification state.
func (obj *Refreshable) SetRefresh(b bool) {
	obj.refresh = b
}

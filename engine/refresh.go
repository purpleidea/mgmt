// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package engine

// RefreshableRes is the interface a resource must implement to support refresh
// notifications. Default implementations for all of the methods declared in
// this interface can be obtained for your resource by anonymously adding the
// traits.Refreshable struct to your resource implementation.
type RefreshableRes interface {
	Res // implement everything in Res but add the additional requirements

	// Refresh returns the refresh notification state.
	Refresh() bool

	// SetRefresh sets the refresh notification state.
	SetRefresh(bool)
}

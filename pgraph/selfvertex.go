// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package pgraph

// SelfVertex is a vertex that stores a graph pointer to the graph that it's on.
// This is useful if you want to pass around a graph with a vertex cursor on it.
type SelfVertex struct {
	Name  string
	Graph *Graph // it's up to you to manage the cursor safety
}

// String is a required method of the Vertex interface that we must fulfill.
func (obj *SelfVertex) String() string {
	return obj.Name
}

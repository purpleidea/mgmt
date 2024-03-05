// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package engine

import (
	"sort"
)

// ResourceSlice is a linear list of resources. It can be sorted.
type ResourceSlice []Res

func (rs ResourceSlice) Len() int           { return len(rs) }
func (rs ResourceSlice) Swap(i, j int)      { rs[i], rs[j] = rs[j], rs[i] }
func (rs ResourceSlice) Less(i, j int) bool { return rs[i].String() < rs[j].String() }

// Sort the list of resources and return a copy without modifying the input.
func Sort(rs []Res) []Res {
	resources := []Res{}
	for _, r := range rs { // copy
		resources = append(resources, r)
	}
	sort.Sort(ResourceSlice(resources))
	return resources
	// sort.Sort(ResourceSlice(rs)) // this is wrong, it would modify input!
	//return rs
}

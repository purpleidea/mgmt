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

package util

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/types"
)

// HasDuplicateTypes returns an error if the list of types is not unique.
func HasDuplicateTypes(typs []*types.Type) error {
	// FIXME: do this comparison in < O(n^2) ?
	for i, ti := range typs {
		for j, tj := range typs {
			if i == j {
				continue // don't compare to self
			}
			if ti.Cmp(tj) == nil {
				return fmt.Errorf("duplicate type of %+v found", ti)
			}
		}
	}
	return nil
}

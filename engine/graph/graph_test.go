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

// +build !root

package graph

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/util/errwrap"
)

func TestMultiErr(t *testing.T) {
	var err error
	e := fmt.Errorf("some error")
	err = errwrap.Append(err, e) // build an error from a nil base
	// ensure that this lib allows us to append to a nil
	if err == nil {
		t.Errorf("missing error")
	}
}

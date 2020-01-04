// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package convert

import (
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func testToInt(t *testing.T, input float64, expected int64) {

	got, err := ToInt([]types.Value{&types.FloatValue{V: input}})
	if err != nil {
		t.Error(err)
		return
	}
	if got.Int() != expected {
		t.Errorf("invalid output, expected %v, got %v", expected, got)
		return
	}
}

func TestToInt1(t *testing.T) {
	testToInt(t, 2.09, 2)

}

func TestToInt2(t *testing.T) {
	testToInt(t, 7.8, 7)
}

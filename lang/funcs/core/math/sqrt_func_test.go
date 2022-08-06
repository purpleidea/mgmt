// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package coremath

import (
	"fmt"
	"math"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
)

func testSqrtSuccess(input, sqrt float64) error {
	inputVal := &types.FloatValue{V: input}

	val, err := Sqrt([]types.Value{inputVal})
	if err != nil {
		return err
	}
	if val.Float() != sqrt {
		return fmt.Errorf("invalid output, expected %f, got %f", sqrt, val.Float())
	}
	return nil
}

func testSqrtError(input float64) error {
	inputVal := &types.FloatValue{V: input}
	_, err := Sqrt([]types.Value{inputVal})
	if err == nil {
		return fmt.Errorf("expected error for input %f, got nil", input)
	}
	return nil
}

func TestSqrtValidInput(t *testing.T) {
	values := map[float64]float64{
		4.0:  2.0,
		16.0: 4.0,
		2.0:  math.Sqrt(2.0),
	}

	for input, sqrt := range values {
		if err := testSqrtSuccess(input, sqrt); err != nil {
			t.Error(err)
		}
	}
}

func TestSqrtInvalidInput(t *testing.T) {
	values := []float64{-1.0}

	for _, input := range values {
		if err := testSqrtError(input); err != nil {
			t.Error(err)
		}
	}
}

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

//go:build !root

package local

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

func TestWrite(t *testing.T) {
	tmpdir := fmt.Sprintf("%s/", t.TempDir()) // gets cleaned up at end, new dir for each call
	key := "test1"
	value := 42
	if err := valueWrite(context.Background(), tmpdir, key, value); err != nil {
		t.Errorf("error: %+v", err)
		return
	}

	if val, err := valueRead(context.Background(), tmpdir, key); err != nil {
		t.Errorf("error: %+v", err)
		return
	} else if !reflect.DeepEqual(value, val) {
		t.Errorf("error: not equal: %+v != %+v", val, value)
		//return
	}

	if err := valueRemove(context.Background(), tmpdir, key); err != nil {
		t.Errorf("error: %+v", err)
		//return
	}
}

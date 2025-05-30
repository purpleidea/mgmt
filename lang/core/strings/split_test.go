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

package corestrings

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

func testSplit(input, sep string, output []string) error {
	inputVal, sepVal := &types.StrValue{V: input}, &types.StrValue{V: sep}

	val, err := Split(context.Background(), []types.Value{inputVal, sepVal})
	if err != nil {
		return err
	}
	listVal, ok := val.(*types.ListValue)
	if !ok {
		return fmt.Errorf("split did not return a list")
	}
	for _, segment := range output {
		if _, ok := listVal.Contains(&types.StrValue{V: segment}); !ok {
			return fmt.Errorf("output does not contained expected segment %s", segment)
		}
	}
	for _, segment := range listVal.V {
		if !util.StrInList(segment.Str(), output) {
			return fmt.Errorf("output contains unexpected segment %s", segment.Str())
		}
	}
	return nil
}

func TestSplit(t *testing.T) {
	values := map[string][]string{
		"hello,world": {"hello", "world"},
		"hello world": {"hello world"},
		"hello;world": {"hello;world"},
	}

	for input, output := range values {
		if err := testSplit(input, ",", output); err != nil {
			t.Error(err)
		}
	}
}

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

package corestrings

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
)

func testSplit(input, sep string, output []string) error {
	inputVal, sepVal := &types.StrValue{V: input}, &types.StrValue{V: sep}

	val, err := Split([]types.Value{inputVal, sepVal})
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

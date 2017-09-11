// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

package resources

import (
	"os/user"
	"testing"
)

func TestExpandHome(t *testing.T) {
	usr, _ := user.Current()
	var expandHomeTests = []struct {
		path     string
		expanded string
	}{
		{"/some/random/path", "/some/random/path"},
		{"~/", usr.HomeDir},
		{"~/some/path", usr.HomeDir + "/some/path"},
		{"~" + usr.Username + "/", usr.HomeDir},
		{"~" + usr.Username + "/some/path", usr.HomeDir + "/some/path"},
	}

	for _, test := range expandHomeTests {
		actual, err := expandHome(test.path)
		if err != nil {
			t.Error(err)
		}

		if actual != test.expanded {
			t.Errorf("expandHome(%s): expected %s, actual %s", test.path, test.expanded, actual)
		}
	}
}

func TestNumToAlpha(t *testing.T) {
	var numToAlphaTests = []struct {
		number int
		result string
	}{
		{0, "a"},
		{25, "z"},
		{26, "aa"},
		{27, "ab"},
		{702, "aaa"},
		{703, "aab"},
		{63269, "cool"},
	}

	for _, test := range numToAlphaTests {
		actual := numToAlpha(test.number)
		if actual != test.result {
			t.Errorf("numToAlpha(%d): expected %s, actual %s", test.number, test.result, actual)
		}
	}
}

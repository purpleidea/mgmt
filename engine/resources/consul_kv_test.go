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

package resources

import "testing"

func TestParseConsulName(t *testing.T) {
	if s, a, k := parseConsulName("test"); s != "" || a != "" || k != "test" {
		t.Errorf("unexpected output while parsing test: %s, %s, %s", s, a, k)
	}
	if s, a, k := parseConsulName("http://127.0.0.1:8500/test"); s != "http" || a != "127.0.0.1:8500" || k != "/test" {
		t.Errorf("unexpected output while parsing 127.0.0.1:8500/test: %s, %s, %s", s, a, k)
	}
}

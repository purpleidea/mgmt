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

package resources

import (
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

func createConsulRes(name string) *ConsulKVRes {
	r, err := engine.NewNamedResource("consul:kv", name)
	if err != nil {
		panic(fmt.Sprintf("could not create resource: %+v", err))
	}

	res := r.(*ConsulKVRes) // if this panics, the test will panic
	return res
}

func TestParseConsulName(t *testing.T) {
	n1 := "test"
	r1 := createConsulRes(n1)
	if s, a, k := r1.inputParser(); s != "" || a != "" || k != "test" {
		t.Errorf("unexpected output while parsing `%s`: %s, %s, %s", n1, s, a, k)
	}

	n2 := "http://127.0.0.1:8500/test"
	r2 := createConsulRes(n2)
	if s, a, k := r2.inputParser(); s != "http" || a != "127.0.0.1:8500" || k != "/test" {
		t.Errorf("unexpected output while parsing `%s`: %s, %s, %s", n2, s, a, k)
	}

	n3 := "http://127.0.0.1:8500/test"
	r3 := createConsulRes(n3)
	r3.Scheme = "https"
	r3.Address = "example.com"
	if s, a, k := r3.inputParser(); s != "https" || a != "example.com" || k != "/test" {
		t.Errorf("unexpected output while parsing `%s`: %s, %s, %s", n3, s, a, k)
	}

	n4 := "http:://127.0.0.1..5:8500/test" // wtf, url.Parse is on drugs...
	r4 := createConsulRes(n4)
	//if s, a, k := r4.inputParser(); s != "" || a != "" || k != n4 { // what i really expect
	if s, a, k := r4.inputParser(); s != "http" || a != "" || k != "" { // what i get
		t.Errorf("unexpected output while parsing `%s`: %s, %s, %s", n4, s, a, k)
	}

	n5 := "http://127.0.0.1:8500/test" // whatever, it's ignored
	r5 := createConsulRes(n3)
	r5.Key = "some key"
	if s, a, k := r5.inputParser(); s != "" || a != "" || k != "some key" {
		t.Errorf("unexpected output while parsing `%s`: %s, %s, %s", n5, s, a, k)
	}
}

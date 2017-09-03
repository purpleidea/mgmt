// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"testing"
)

func TestHttpWatch1(t *testing.T) {
	r1 := &HTTPRes{
		BaseRes: BaseRes{
			Name:       "msg1",
			Kind:       "http",
			MetaParams: DefaultMetaParams,
		},
		URL: "http://localhost:12345/hello",
	}
	r1.Init()
	r1.Setup(nil, r1, r1)

	err := r1.Watch()
	if err != nil {
		t.Error(err)
	}
}

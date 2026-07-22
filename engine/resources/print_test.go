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

package resources

import (
	"context"
	"fmt"
	"testing"

	"github.com/purpleidea/mgmt/engine"
)

func TestPrintEmptyMessageFirstRun(t *testing.T) {
	logs := []string{}
	obj := &PrintRes{}
	if err := obj.Init(&engine.Init{
		Logf: func(format string, v ...interface{}) {
			logs = append(logs, fmt.Sprintf(format, v...))
		},
		Recv: func() map[string]*engine.Send {
			return map[string]*engine.Send{}
		},
		Refresh: func() bool { return false },
	}); err != nil {
		t.Fatalf("func Init failed: %v", err)
	}
	obj.evch = make(chan struct{}, 1)
	defer obj.Cleanup()

	checkOK, err := obj.CheckApply(context.Background(), true)
	if err != nil {
		t.Fatalf("func CheckApply failed: %v", err)
	}
	if checkOK {
		t.Errorf("func CheckApply returned true on the first run")
	}

	if len(logs) != 1 || logs[0] != "<empty>" {
		t.Fatalf("unexpected logs: %#v", logs)
	}
}

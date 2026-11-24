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

//go:build !root

package resources

import (
	"testing"

	"github.com/purpleidea/mgmt/util/systemd"

	"github.com/godbus/dbus/v5"
)

func TestSvcUnitName(t *testing.T) {
	tests := []struct {
		name string // the name of the svc resource
		unit string // what systemd calls it
		path string // its object path on the bus
	}{
		{"sshd", "sshd.service", "sshd_2eservice"},
		{"1foo", "1foo.service", "_31foo_2eservice"}, // only the path escapes it
		{"avahi-daemon", "avahi-daemon.service", "avahi_2ddaemon_2eservice"},
		{"getty@tty1", "getty@tty1.service", "getty_40tty1_2eservice"},
		{`foo\x21`, `foo\x21.service`, "foo_5cx21_2eservice"},
		{"foo!", `foo\x21.service`, "foo_5cx21_2eservice"}, // like systemctl
		{"foo/bar", "foo-bar.service", "foo_2dbar_2eservice"},
		{
			"🍑", // four bytes
			`\xf0\x9f\x8d\x91.service`,
			"_5cxf0_5cx9f_5cx8d_5cx91_2eservice",
		},
		{
			"🪣", // four bytes
			`\xf0\x9f\xaa\xa3.service`,
			"_5cxf0_5cx9f_5cxaa_5cxa3_2eservice",
		},
	}
	for _, tc := range tests {
		res := &SvcRes{}
		res.SetName(tc.name)
		unit := res.svc()
		if unit != tc.unit {
			t.Errorf("svc %q: got unit %q, expected %q", tc.name, unit, tc.unit)
		}
		expected := dbus.ObjectPath("/org/freedesktop/systemd1/unit/" + tc.path)
		if path := systemd.UnitPath(unit); path != expected {
			t.Errorf("svc %q: got path %q, expected %q", tc.name, path, expected)
		}

		// A pkg which ships this unit file gets an edge to us.
		file := "/usr/lib/systemd/system/" + tc.unit
		names := ReturnSvcInFileList([]string{file})
		if len(names) != 1 {
			t.Errorf("svc %q: got names %q from %s", tc.name, names, file)
			continue
		}
		pkg := &SvcUID{name: names[0]}
		if !pkg.IFF(res.UIDs()[0]) {
			t.Errorf("svc %q: no edge from a pkg with %s", tc.name, file)
		}
	}
}

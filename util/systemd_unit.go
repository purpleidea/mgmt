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

package util

import (
	"fmt"
)

// UnitData is the data struct used to build a systemd unit file. This isn't an
// exhaustive representation of what's possible, but is meant to handle most
// common cases. Alternatively we could have used the
// github.com/coreos/go-systemd/v22/unit library, but it didn't provide much
// value. More documentation on these fields can be seen at:
// https://www.freedesktop.org/software/systemd/man/latest/systemd.service.html
type UnitData struct {
	// Description field for the unit file.
	Description string

	// Documentation field for the unit file.
	Documentation string

	// After makes this happen after that target is ready. Usually you want
	// "network.target" or "systemd-networkd.service" here I think.
	After []string

	// Type is the type of service to run, such as "oneshot".
	Type string

	// ExecStart is the main command to run.
	ExecStart string

	// RestartSec is the number of seconds to wait between restarts.
	RestartSec string

	// Restart is the restart policy. Usually you want "always".
	Restart string

	// RemainAfterExit can be set to true to make it look like the service
	// is still active if it exits successfully.
	RemainAfterExit bool

	// StandardOutput specifies what happens with stdout.
	StandardOutput string

	// StandardError specifies what happens with stderr.
	StandardError string

	// WantedBy makes this a dependency of this target. Usually you want
	// "multi-user.target" here.
	WantedBy []string
}

// Template uses the UnitData to build a systemd unit file. This API is subject
// to change, don't assume the output is stable either.
// TODO: Should this return an error or not?
func (obj *UnitData) Template() (string, error) {
	data := ""
	data += "[Unit]\n"
	if obj.Description != "" {
		data += fmt.Sprintf("Description=%s\n", obj.Description)
	}
	if obj.Documentation != "" {
		data += fmt.Sprintf("Documentation=%s\n", obj.Documentation)
	}
	for _, x := range obj.After {
		if x == "" {
			continue
		}
		data += fmt.Sprintf("After=%s\n", x)
	}

	data += "\n"
	data += "[Service]\n"
	if obj.Type != "" {
		data += fmt.Sprintf("Type=%s\n", obj.Type)
	}
	if obj.ExecStart != "" {
		data += fmt.Sprintf("ExecStart=%s\n", obj.ExecStart)
	}
	if obj.RestartSec != "" {
		data += fmt.Sprintf("RestartSec=%s\n", obj.RestartSec)
	}
	if obj.Restart != "" {
		data += fmt.Sprintf("Restart=%s\n", obj.Restart)
	}

	if obj.RemainAfterExit {
		data += "RemainAfterExit=yes\n"
	}
	if obj.StandardOutput != "" {
		data += fmt.Sprintf("StandardOutput=%s\n", obj.StandardOutput)
	}
	if obj.StandardError != "" {
		data += fmt.Sprintf("StandardError=%s\n", obj.StandardError)
	}

	data += "\n"
	data += "[Install]\n"
	for _, x := range obj.WantedBy {
		if x == "" {
			continue
		}
		data += fmt.Sprintf("WantedBy=%s\n", x)
	}

	return data, nil
}

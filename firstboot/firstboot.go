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

// Package firstboot provides a service that runs scripts once on first boot.
// You can easily test this with:
//
// mkdir /tmp/mgmt-firstboot/ ; echo 'echo hello' > /tmp/mgmt-firstboot/foo.sh
// ./mgmt firstboot start --lock-file-path=/tmp/flock.lock \
// --scripts-dir=/tmp/mgmt-firstboot/ --logging-dir=/tmp/
package firstboot

import (
	"context"
)

// API is the simple interface we expect for any setup items.
type API interface {
	// Main runs everything for this setup item.
	Main(context.Context) error
}

// Config is a struct of all the configuration values which are shared by all of
// the setup utilities. By including this as a separate struct, it can be used
// as part of the API if we want.
type Config struct {
	//Foo string `arg:"--foo,env:MGMT_FIRSTBOOT_FOO" help:"Foo..."` // TODO: foo
}

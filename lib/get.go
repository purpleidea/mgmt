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

package lib

import (
	"fmt"
	"log"

	"github.com/purpleidea/mgmt/gapi"

	"github.com/urfave/cli"
)

// get is the cli target to run code/import downloads.
func get(c *cli.Context, name string, gapiObj gapi.GAPI) error {
	cliContext := c.Lineage()[1]
	if cliContext == nil {
		return fmt.Errorf("could not get cli context")
	}

	program, version := safeProgram(c.App.Name), c.App.Version
	var flags Flags
	var debug bool
	if val, exists := c.App.Metadata["flags"]; exists {
		if f, ok := val.(Flags); ok {
			flags = f
			debug = flags.Debug
		}
	}
	hello(program, version, flags) // say hello!

	gettable, ok := gapiObj.(gapi.GettableGAPI)
	if !ok {
		// this is a programming bug as this should not get called...
		return fmt.Errorf("the `%s` GAPI does not implement: %s", name, gapi.CommandGet)
	}

	getInfo := &gapi.GetInfo{
		CliContext: c, // don't pass in the parent context

		Noop:   cliContext.Bool("noop"),
		Sema:   cliContext.Int("sema"),
		Update: cliContext.Bool("update"),

		Debug: debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			log.Printf(name+": "+format, v...)
		},
	}

	if err := gettable.Get(getInfo); err != nil {
		return err // no need to errwrap here
	}

	log.Printf("%s: success!", name)
	return nil
}

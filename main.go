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

package main

import (
	_ "embed"
	"fmt"
	"os"

	mgmt "github.com/purpleidea/mgmt/lib"
)

// These constants are some global variables that are used throughout the code.
const (
	Debug   = false // add additional log messages
	Verbose = false // add extra log message output
)

// set at compile time
var (
	program string
	version string
)

//go:embed COPYING
var copying string

func main() {
	flags := mgmt.Flags{
		Debug:   Debug,
		Verbose: Verbose,
	}
	cliArgs := &mgmt.CLIArgs{
		Program: program,
		Version: version,
		Copying: copying,
		Flags:   flags,
	}
	if err := mgmt.CLI(cliArgs); err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
}

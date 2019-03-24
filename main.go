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

package main

import (
	"fmt"
	"os"

	mgmt "github.com/purpleidea/mgmt/lib"
)

// These constants are some global variables that are used throughout the code.
const (
	Debug   = false // add additional log messages
	Trace   = false // add execution flow log messages
	Verbose = false // add extra log message output
)

// set at compile time
var (
	program string
	version string
)

func main() {
	flags := mgmt.Flags{
		Debug:   Debug,
		Trace:   Trace,
		Verbose: Verbose,
	}
	if err := mgmt.CLI(program, version, flags); err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
}

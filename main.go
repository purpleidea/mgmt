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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/purpleidea/mgmt/cli"
	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"go.etcd.io/etcd/server/v3/etcdmain"
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
	// embed etcd completely
	if len(os.Args) >= 2 && os.Args[1] == "etcd" {
		args := []string{}
		for i, s := range os.Args {
			if i == 0 { // pop off our argv[0] and let `etcd` be it
				continue
			}
			args = append(args, s)
		}
		etcdmain.Main(args) // this will os.Exit
		return              // for safety
	}

	cliArgs := &cli.CLIArgs{
		Program: program,
		Version: version,
		Copying: copying,
		Flags: cliUtil.Flags{
			Debug:   Debug,
			Verbose: Verbose,
		},
	}
	if err := cli.CLI(cliArgs); err != nil {
		fmt.Println(err)
		os.Exit(1)
		return
	}
}

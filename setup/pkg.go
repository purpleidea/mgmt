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

package setup

import (
	"context"
	"fmt"
	"strings"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util"
	distroUtil "github.com/purpleidea/mgmt/util/distro"
)

// Pkg is the standalone entry program for the pkg setup component.
type Pkg struct {
	*cliUtil.SetupPkgArgs // embedded config
	Config                // embedded Config

	// Program is the name of this program, usually set at compile time.
	Program string

	// Version is the version of this program, usually set at compile time.
	Version string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Main runs everything for this setup item.
func (obj *Pkg) Main(ctx context.Context) error {
	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Run(ctx); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Pkg) Validate() error {
	if obj == nil {
		return fmt.Errorf("data is nil")
	}
	if obj.Program == "" {
		return fmt.Errorf("program is empty")
	}
	if obj.Version == "" {
		return fmt.Errorf("version is empty")
	}

	if obj.SetupPkgArgs.Distro == "" {
		return fmt.Errorf("distro is empty")
	}

	return nil
}

// Run performs the desired actions. This generates a list of bash commands to
// run since we might not be able to run this binary to install these packages!
// The output (stdout) of this command can be run from a shell.
func (obj *Pkg) Run(ctx context.Context) error {
	cmdName := ""

	packages, exists := distroUtil.ToBootstrapPackages(obj.SetupPkgArgs.Distro)
	if !exists {
		return fmt.Errorf("unknown distro")
	}

	// TODO: Consider moving cmdName into the util/distro package.
	if obj.SetupPkgArgs.Distro == "fedora" {
		cmdName = "/usr/bin/dnf --assumeyes install"
	}
	if obj.SetupPkgArgs.Distro == "debian" {
		cmdName = "/usr/bin/apt --yes install"
	}

	if cmdName == "" {
		return fmt.Errorf("no command name found")
	}
	if len(packages) == 0 {
		return nil // nothing to do
	}

	cmdArgs := []string{}
	cmdArgs = append(cmdArgs, packages...)

	if !obj.SetupPkgArgs.Exec { // print, don't exec
		cmd := ""
		if obj.SetupPkgArgs.Sudo {
			cmd += "sudo" + " "
		}
		cmd += cmdName + " "

		cmd += strings.Join(cmdArgs, " ")

		fmt.Printf("%s\n", cmd)
		return nil
	}

	// Split off any bonus elements to the command...
	realCmdName := strings.Split(cmdName, " ")
	realCmdArgs := []string{}
	realCmdArgs = append(realCmdArgs, realCmdName[1:]...)
	realCmdArgs = append(realCmdArgs, cmdArgs...)
	opts := &util.SimpleCmdOpts{
		Debug: obj.Debug,
		Logf:  obj.Logf,
	}
	return util.SimpleCmd(ctx, realCmdName[0], realCmdArgs, opts)
}

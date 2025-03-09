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
	"os"
	"strings"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util"
)

// Firstboot is the standalone entry program for the firstboot setup component.
type Firstboot struct {
	*cliUtil.SetupFirstbootArgs // embedded config
	Config                      // embedded Config

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
func (obj *Firstboot) Main(ctx context.Context) error {
	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Run(ctx); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Firstboot) Validate() error {
	if obj == nil {
		return fmt.Errorf("data is nil")
	}
	if obj.Program == "" {
		return fmt.Errorf("program is empty")
	}
	if obj.Version == "" {
		return fmt.Errorf("version is empty")
	}

	return nil
}

// Run performs the desired actions. This templates and installs a systemd
// service and enables and starts it if so desired.
func (obj *Firstboot) Run(ctx context.Context) error {
	cmdNameSystemctl := "/usr/bin/systemctl"
	opts := &util.SimpleCmdOpts{
		Debug: obj.Debug,
		Logf:  obj.Logf,
	}

	if obj.SetupFirstbootArgs.Mkdir {
		// TODO: Should we also make LoggingDir and LockFilePath's dir?
		if s := obj.SetupFirstbootArgs.ScriptsDir; s != "" {
			if err := os.MkdirAll(s, 0755); err != nil {
				return err
			}
			obj.Logf("mkdir: %s", s)
		}
	}

	if obj.SetupFirstbootArgs.Install {
		binaryPath := "/usr/bin/mgmt" // default
		if s := obj.SetupFirstbootArgs.BinaryPath; s != "" {
			binaryPath = s
		}

		args := []string{}
		if s := obj.SetupFirstbootArgs.LockFilePath; s != "" {
			arg := fmt.Sprintf("--lock-file-path=%s", s)
			args = append(args, arg)
		}

		if s := obj.SetupFirstbootArgs.ScriptsDir; s != "" {
			arg := fmt.Sprintf("--scripts-dir=%s", s)
			args = append(args, arg)
		}

		if s := obj.SetupFirstbootArgs.DoneDir; s != "" {
			arg := fmt.Sprintf("--done-dir=%s", s)
			args = append(args, arg)
		}

		if s := obj.SetupFirstbootArgs.LoggingDir; s != "" {
			arg := fmt.Sprintf("--logging-dir=%s", s)
			args = append(args, arg)
		}

		unit := &util.UnitData{
			Description:   "Mgmt firstboot service",
			Documentation: "https://github.com/purpleidea/mgmt/",
			After:         []string{"network.target"}, // TODO: systemd-networkd.service ?

			Type:      "oneshot",
			ExecStart: fmt.Sprintf("%s firstboot start %s", binaryPath, strings.Join(args, " ")),

			RemainAfterExit: true,
			StandardOutput:  "journal+console",
			StandardError:   "inherit",

			WantedBy: []string{"multi-user.target"},
		}
		unitData, err := unit.Template()
		if err != nil {
			return err
		}
		unitPath := "/etc/systemd/system/mgmt-firstboot.service"

		if err := os.WriteFile(unitPath, []byte(unitData), 0644); err != nil {
			return err
		}
		obj.Logf("wrote file to: %s", unitPath)
	}

	if obj.SetupFirstbootArgs.Start {
		cmdArgs := []string{"start", "mgmt-firstboot.service"}
		if err := util.SimpleCmd(ctx, cmdNameSystemctl, cmdArgs, opts); err != nil {
			return err
		}
	}

	if obj.SetupFirstbootArgs.Enable {
		cmdArgs := []string{"enable", "mgmt-firstboot.service"}
		if err := util.SimpleCmd(ctx, cmdNameSystemctl, cmdArgs, opts); err != nil {
			return err
		}
	}

	return nil
}

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

// Svc is the standalone entry program for the svc setup component.
type Svc struct {
	*cliUtil.SetupSvcArgs // embedded config
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
func (obj *Svc) Main(ctx context.Context) error {
	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Run(ctx); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Svc) Validate() error {
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
func (obj *Svc) Run(ctx context.Context) error {
	once := false
	cmdNameSystemctl := "/usr/bin/systemctl"
	opts := &util.SimpleCmdOpts{
		Debug: obj.Debug,
		Logf:  obj.Logf,
	}

	if obj.SetupSvcArgs.NoServer && len(obj.SetupSvcArgs.Seeds) == 0 {
		return fmt.Errorf("--no-server can't be used with zero seeds")
	}

	if obj.SetupSvcArgs.Install {
		binaryPath := "/usr/bin/mgmt" // default
		if s := obj.SetupSvcArgs.BinaryPath; s != "" {
			binaryPath = s
		}

		argv := []string{
			binaryPath,
			"run", // run command
		}

		if s := obj.SetupSvcArgs.SSHURL; s != "" {
			// TODO: validate ssh url? Should be user@server:port
			argv = append(argv, fmt.Sprintf("--ssh-url=%s", s))
		}

		if seeds := obj.SetupSvcArgs.Seeds; len(seeds) > 0 {
			// TODO: validate each seed?
			s := fmt.Sprintf("--seeds=%s", strings.Join(seeds, ","))
			argv = append(argv, s)
		}

		if obj.SetupSvcArgs.NoServer {
			argv = append(argv, "--no-server")
			argv = append(argv, "--no-magic") // XXX: fix this workaround
		}

		argv = append(argv, "empty $OPTS")
		execStart := strings.Join(argv, " ")

		unit := &util.UnitData{
			Description:   "Mgmt configuration management service",
			Documentation: "https://github.com/purpleidea/mgmt/",
			ExecStart:     execStart,
			RestartSec:    "5s",
			Restart:       "always",
			LimitNOFILE:   16384,
			WantedBy:      []string{"multi-user.target"},
		}
		unitData, err := unit.Template()
		if err != nil {
			return err
		}
		unitPath := "/etc/systemd/system/mgmt.service"

		if err := os.WriteFile(unitPath, []byte(unitData), 0644); err != nil {
			return err
		}
		obj.Logf("wrote file to: %s", unitPath)
		once = true
	}

	if obj.SetupSvcArgs.Start {
		cmdArgs := []string{"start", "mgmt.service"}
		if err := util.SimpleCmd(ctx, cmdNameSystemctl, cmdArgs, opts); err != nil {
			return err
		}
		once = true
	}

	if obj.SetupSvcArgs.Enable {
		cmdArgs := []string{"enable", "mgmt.service"}
		if err := util.SimpleCmd(ctx, cmdNameSystemctl, cmdArgs, opts); err != nil {
			return err
		}
		once = true
	}

	if !once {
		return fmt.Errorf("nothing done")
	}
	return nil
}

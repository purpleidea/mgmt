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

package cli

import (
	"context"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/lib"
)

// CheckArgs RunArgs is the CLI parsing structure and type of the parsed result.
// This particular one contains all the common flags for the `check` subcommand.
type CheckArgs struct {
	lib.Config // embedded config (can't be a pointer) https://github.com/alexflint/go-arg/issues/240

	CheckLang *CheckLangArgs `arg:"subcommand:lang" help:"check lang (mcl) payload"`
}

// Run executes the correct subcommand. It errors if there's ever an error. It
// returns true if we did activate one of the subcommands. It returns false if
// we did not. This information is used so that the top-level parser can return
// usage or help information if no subcommand activates. This particular Run is
// the run for the main `check` subcommand. The check command allows validation
// of mcl files.
func (obj *CheckArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	if cmd := obj.CheckLang; cmd != nil {
		// Run the 'run lang' command, but stop after all of the checks.
		la := &cliUtil.LangArgs{
			Input:              cmd.Input,
			Download:           cmd.Download,
			Update:             cmd.Update,
			UnifySolver:        cmd.UnifySolver,
			UnifyOptimizations: cmd.UnifyOptimizations,
			CheckOnly:          true,
			CheckFmt:           !cmd.SkipFmt,
			CheckUnify:         !cmd.SkipUnify,
			Depth:              cmd.Depth,
			Retry:              cmd.Retry,
			ModulePath:         cmd.ModulePath,
		}

		run := RunArgs{
			RunLang: la,
		}
		return run.Run(ctx, data)
	}

	return false, nil
}

// CheckLangArgs is the check lang CLI parsing structure and type of the parsed
// result.
type CheckLangArgs struct {
	// Input is the input mcl code or file path or any input specification.
	Input string `arg:"positional,required"`

	Download bool `arg:"--download" help:"download any missing imports"`
	Update   bool `arg:"--update" help:"update all dependencies to the latest versions"`

	// SkipUnify specifies that the mcl unification check should be skipped.
	// It is used by the check command, which enables that check by default.
	SkipUnify bool `arg:"--skip-unify" help:"skip the mcl type unification"`

	UnifySolver        *string  `arg:"--unify-name" help:"pick a specific unification solver"`
	UnifyOptimizations []string `arg:"--unify-optimizations,separate" help:"list of unification optimizations to request (experts only)"`

	// SkipFmt specifies that the mcl format check should be skipped. It is
	// used by the check command, which enables that check by default.
	SkipFmt bool `arg:"--skip-fmt" help:"skip the mcl format check"`

	Depth int `arg:"--depth" default:"-1" help:"max recursion depth limit (-1 is unlimited)"`

	// The default of 0 means any error is a failure by default.
	Retry int `arg:"--retry" help:"max number of retries (-1 is unlimited)"`

	ModulePath string `arg:"--module-path,env:MGMT_MODULE_PATH" help:"choose the modules path (absolute)"`
}

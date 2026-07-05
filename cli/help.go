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
	"os"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/alexflint/go-arg"
)

// HelpArgs is the CLI parsing structure and type of the parsed result. This
// particular one displays the same top-level help that bare `mgmt` does.
type HelpArgs struct{}

// Run displays the standard top-level help output.
func (obj *HelpArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	args := Args{}
	args.version = data.Version // copy this in
	args.description = data.Tagline

	config := arg.Config{
		Program: data.Program,
	}
	parser, err := arg.NewParser(config, &args)
	if err != nil {
		// programming error
		return false, errwrap.Wrapf(err, "cli config error")
	}
	parser.WriteHelp(os.Stdout)
	return true, nil
}

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

// Package cli handles all of the core command line parsing. It's the first
// entry point after the real main function, and it imports and runs our core
// "lib".
package cli

import (
	"context"
	"fmt"
	"os"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/alexflint/go-arg"
)

// CLI is the entry point for using mgmt normally from the CLI.
func CLI(ctx context.Context, data *cliUtil.Data) error {
	// test for sanity
	if data == nil {
		return fmt.Errorf("this CLI was not run correctly")
	}
	if data.Program == "" || data.Version == "" {
		return fmt.Errorf("program was not compiled correctly")
	}
	if data.Copying == "" {
		return fmt.Errorf("program copyrights were removed, can't run")
	}

	args := Args{}
	args.version = data.Version // copy this in
	args.description = data.Tagline

	config := arg.Config{
		Program: data.Program,
	}
	parser, err := arg.NewParser(config, &args)
	if err != nil {
		// programming error
		return errwrap.Wrapf(err, "cli config error")
	}
	err = parser.Parse(data.Args[1:]) // XXX: args[0] needs to be dropped
	if err == arg.ErrHelp {
		parser.WriteHelp(os.Stdout)
		return nil
	}
	if err == arg.ErrVersion {
		fmt.Printf("%s\n", data.Version) // byon: bring your own newline
		return nil
	}
	if err != nil {
		//parser.WriteHelp(os.Stdout) // TODO: is doing this helpful?
		return cliUtil.CliParseError(err) // consistent errors
	}

	// display the license
	if args.License {
		fmt.Printf("%s", data.Copying) // file comes with a trailing nl
		return nil
	}

	if ok, err := args.Run(ctx, data); err != nil {
		return err
	} else if ok { // did we activate one of the commands?
		return nil
	}

	// print help if no subcommands are set
	parser.WriteHelp(os.Stdout)

	return nil
}

// Args is the CLI parsing structure and type of the parsed result. This
// particular struct is the top-most one.
type Args struct {
	// XXX: We cannot have both subcommands and a positional argument.
	// XXX: I think it's a bug of this library that it can't handle argv[0].
	//Argv0 string `arg:"positional"`

	License bool `arg:"--license" help:"display the license and exit"`

	RunCmd *RunArgs `arg:"subcommand:run" help:"run code on this machine"`

	DeployCmd *DeployArgs `arg:"subcommand:deploy" help:"deploy code into a cluster"`

	// This never runs, it gets preempted in the real main() function.
	// XXX: Can we do it nicely with the new arg parser? can it ignore all args?
	EtcdCmd *EtcdArgs `arg:"subcommand:etcd" help:"run standalone etcd"`

	// version is a private handle for our version string.
	version string `arg:"-"` // ignored from parsing

	// description is a private handle for our description string.
	description string `arg:"-"` // ignored from parsing
}

// Version returns the version string. Implementing this signature is part of
// the API for the cli library.
func (obj *Args) Version() string {
	return obj.version
}

// Description returns a description string. Implementing this signature is part
// of the API for the cli library.
func (obj *Args) Description() string {
	return obj.description
}

// Run executes the correct subcommand. It errors if there's ever an error. It
// returns true if we did activate one of the subcommands. It returns false if
// we did not. This information is used so that the top-level parser can return
// usage or help information if no subcommand activates.
func (obj *Args) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {
	if cmd := obj.RunCmd; cmd != nil {
		return cmd.Run(ctx, data)
	}

	if cmd := obj.DeployCmd; cmd != nil {
		return cmd.Run(ctx, data)
	}

	// NOTE: we could return true, fmt.Errorf("...") if more than one did
	return false, nil // nobody activated
}

// EtcdArgs is the CLI parsing structure and type of the parsed result. This
// particular one is empty because the `etcd` subcommand is preempted in the
// real main() function.
type EtcdArgs struct{}

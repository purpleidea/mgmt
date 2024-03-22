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

// Package entry provides an API to kicking off the initial binary execution.
// The functions and data structures in this library can be used to package your
// own custom entry point as a custom application.
// TODO: Should this be nested inside of lang/ or can it be used for all GAPI's?
package entry

import (
	"context"
	"fmt"
	"os"

	"github.com/purpleidea/mgmt/cli"
	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/alexflint/go-arg"
)

// registeredData is the single "registered" entry point that we built with. You
// cannot have more than one currently.
// TODO: In the future we could have more than one registered and each could
// appear under a top-level "embedded" subcommand if we decided not to have a
// "default" singleton registered.
var registeredData *Data

// Register takes input data and stores it for lookup by the top-level main
// function. Register is commonly called in the init() method of the module that
// defined it, which happens at program startup. Build flags should be used to
// determine which Register gets to run. Only one entry can be registered at a
// time. There is no matching Unregister function at this time.
func Register(data *Data) {
	if registeredData != nil {
		panic("an entry is already registered")
	}
	if err := data.Validate(); err != nil {
		panic(err)
	}

	registeredData = data
}

// Data is what a prospective standalone entry program must specify to our API.
type Data struct {
	// Program is the name of this program, usually set at compile time.
	Program string

	// Version is the version of this program, usually set at compile time.
	Version string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})

	// Args is the CLI struct to use. This takes the format of the go-arg
	// API. Keep in mind that these values will be added on to the normal
	// run subcommand with frontend that is chosen. Make sure you don't add
	// anything that would conflict with that. Of note, a new subcommand is
	// probably not what you want. To do more complicated things, you will
	// need to implement a different custom API with Customizable.
	Args interface{}

	// Frontend is the name of the GAPI to run.
	Frontend string

	// Top is the initial input or code to run.
	Top []byte

	// Customizable is an additional API you can implement to have tighter
	// control over how the entry executes mgmt.
	Custom Customizable
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Data) Validate() error {
	if obj == nil {
		return fmt.Errorf("data is nil")
	}
	if obj.Program == "" {
		return fmt.Errorf("program is empty")
	}
	if obj.Version == "" {
		return fmt.Errorf("version is empty")
	}
	if _, err := arg.NewParser(arg.Config{}, obj.Args); err != nil { // sanity check
		return errwrap.Wrapf(err, "invalid args cli struct")
	}
	if obj.Frontend == "" {
		return fmt.Errorf("frontend is empty")
	}
	if len(obj.Top) == 0 {
		return fmt.Errorf("top is empty")
	}
	//if obj.Custom == nil { // this is allowed!
	//	return fmt.Errorf("custom is nil")
	//}

	return nil
}

// Lookup returns the runner that implements the complex plumbing to kick off
// the run. If one has not been registered, then this will error.
func Lookup() (*Runner, error) {
	if registeredData == nil {
		return nil, fmt.Errorf("could not find a registered entry")
	}

	return &Runner{
		data: registeredData, // *Data
	}, nil
}

// runnerArgs are some default args that get forced into the arg parser.
type runnerArgs struct {
	License bool `arg:"--license" help:"display the license and exit"`

	// version is a private handle for our version string.
	version string `arg:"-"` // ignored from parsing

	// description is a private handle for our description string.
	description string `arg:"-"` // ignored from parsing
}

// Version returns the version string. Implementing this signature is part of
// the API for the cli library.
func (obj *runnerArgs) Version() string {
	return obj.version
}

// Description returns a description string. Implementing this signature is part
// of the API for the cli library.
func (obj *runnerArgs) Description() string {
	return obj.description
}

// Runner implements the complex plumbing that kicks off the run. The top-level
// main function should call our Run method. This is all private because it
// should only get returned by the Lookup method and used as-is.
type Runner struct {
	data *Data
}

// CLI is the entry point for using any embedded package from the CLI. It is
// used as the main entry point from the top-level main function and kicks-off
// the CLI parser.
//
// XXX: This function is analogous to the cli/cli.go:CLI() function. Could it be
// shared with what's there already or extended to get used for the method too?
func (obj *Runner) CLI(ctx context.Context, data *cliUtil.Data) error {
	// obj.data comes from what the user Registered(): trust this less
	// cli.data comes from what the mgmt compiler specified: trust this more

	// test for sanity
	if data == nil {
		return fmt.Errorf("this CLI was not run correctly")
	}
	if data.Program == "" || data.Version == "" {
		return fmt.Errorf("program was not compiled correctly, see Makefile")
	}
	if data.Copying == "" {
		return fmt.Errorf("program copyrights were removed, can't run")
	}

	// TODO: If obj.data has any special API's for getting program name,
	// version, or anything else in particular, we can use those values to
	// override what we get at compile time from main.main() that comes in
	// here in our *cliUtil.Data input.

	config := arg.Config{
		Program: data.Program,
	}

	runnerArgs := &runnerArgs{}
	runnerArgs.version = data.Version // copy this in
	//runnerArgs.description = data.Tagline

	runArgs := &cli.RunArgs{} // This entry API is based on the `run` cli!

	// You can pass in more than one struct and they are all used. Neat!
	// XXX: This generates sub-optimal help text because we mask the subcmd
	// XXX: Improve the arg parser library so that we can produce good help
	parser, err := arg.NewParser(config, runnerArgs, runArgs, obj.data.Args) // this is the struct
	if err != nil {
		return err
	}

	osArgs := data.Args[1:] // XXX: args[0] needs to be dropped
	if obj.data.Custom != nil {
		osArgs = append(osArgs, obj.data.Frontend) // HACK: add on the frontend sub-command name
		osArgs = append(osArgs, "''")              // HACK: add on fake input for sub-command
	}
	err = parser.Parse(osArgs)
	if err == arg.ErrHelp {
		parser.WriteHelp(os.Stdout)
		return nil
	}
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
	if runnerArgs.License {
		fmt.Printf("%s", data.Copying) // file comes with a trailing nl
		return nil
	}

	var args interface{}
	if cmd := runArgs.RunEmpty; cmd != nil {
		//name = cliUtil.LookupSubcommand(runArgs, cmd) // "empty"
		args = cmd
		// nothing to overwrite here
	}
	if cmd := runArgs.RunLang; cmd != nil {
		//name = cliUtil.LookupSubcommand(runArgs, cmd) // "lang"
		args = cmd
		//fmt.Printf("Input: %+v\n", cmd.Input) // :(
		cmd.Input = string(obj.data.Top) // overwrite
	}
	if cmd := runArgs.RunYaml; cmd != nil {
		//name = cliUtil.LookupSubcommand(runArgs, cmd) // "yaml"
		args = cmd
		cmd.Input = string(obj.data.Top) // overwrite
	}
	_ = args

	//debug := data.Flags.Debug // this one comes from main
	debug := obj.data.Debug // this one comes from entry
	logf := func(format string, v ...interface{}) {
		//data.Flags.Logf(obj.data.Program+": "+format, v...)
		obj.data.Logf(obj.data.Program+": "+format, v...)
	}
	if obj.data.Custom != nil {
		if x, ok := obj.data.Custom.(Initable); ok {
			init := &Init{
				Data:  data,
				Debug: debug,
				Logf:  logf,
				// TODO: add more?
			}
			if err := x.Init(init); err != nil {
				return errwrap.Wrapf(err, "can't init custom struct")
			}
		}

		// The obj.data.Args value is our own Args struct which we can
		// already have access to by holding on to it ourselves when we
		// create it! So no need to pass it back in to ourselves...
		//libConfig, err := obj.data.Custom.Customize(obj.data.Args)
		// Instead pass in something more useful that we don't have!
		// This will contain the parsed result of *lib.Config that the
		// cmdline arg parser already parsed! This function can now
		// modify it based on it's own `args` (obj.data.Args) and pass
		// out an "improved" *lib.Config structure to actually use!
		//libConfig, err := obj.data.Custom.Customize(&runArgs.Config) // TODO: I wish runArgs was a ptr
		// But unbelievably (and more awkwardly) we can do even better
		// by passing in the full runArgs struct (which includes the
		// frontend parsing) and then we can modify any part of that
		// whole thing and return it back. That's what we then can use!
		runArgs, err = obj.data.Custom.Customize(runArgs)
		if err != nil {
			return err
		}
		if runArgs == nil {
			return fmt.Errorf("entry broke the runArgs struct")
		}
		//if libConfig != nil {
		//	runArgs.Config = *libConfig // TODO: I wish runArgs took a ptr
		//}
	}

	if ok, err := runArgs.Run(ctx, data); err != nil {
		return err
	} else if !ok { // did we activate one of the commands?
		return fmt.Errorf("command could not execute")
	}

	return nil
}

// Init is some data and handles to pass in.
type Init struct {
	// Data is the original data that we get from the core compilation.
	Data *cliUtil.Data

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Initable lets us have a way to pass in some data and handles if the struct
// wants them. Implementing this is optional.
type Initable interface {
	Customizable

	// Init passes in some data and handles.
	Init(*Init) error
}

// Customizable is an additional API you can implement to have tighter control
// over how the entry executes mgmt.
// TODO: add an API with: func(arg.Config) (*arg.Parser, error) ?
type Customizable interface {
	// Customize takes in the full parsed struct, and returns the RunArgs
	// that we should use for the run operation.
	// TODO: should the input type be *cli.RunArgs instead?
	Customize(runArgs interface{}) (*cli.RunArgs, error)
	//Customize(args interface{}) (*lib.Config, error)
	//Customize(ctx context.Context, runArgs interface{}) error
}

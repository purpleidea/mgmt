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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/lang/format"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// FmtArgs is the CLI parsing structure and type of the parsed result. This
// particular one contains the flags for the `fmt` subcommand. This command
// recursively formats files by descending through the filesystem hierarchy.
// This is in contrast to the `check` subcommand which examines various aspects
// by traversing the AST. By default this command always formats. This makes it
// ergonomic to run `mgmt check` in your source dir and have everything fixed
// without needing a `Makefile` wrapper like golang needs for `gofmt`. Without
// an input param it starts in the current working directory. If you specify a
// file it will only format that. If you specify a directory it will descend
// into that. If you use the `--test` arg, it only checks and doesn't write. It
// returns 0 if everything is correctly formatted, otherwise non-zero.
type FmtArgs struct {
	Test    bool `arg:"-t,--test" help:"return non-zero if not formatted correctly"`
	Verbose bool `arg:"-v,--verbose" help:"print logs of the common operations"`

	// Input is the input mcl code or file path or any input specification.
	Input string `arg:"positional"`
}

// Run formats the selected mcl input. Return true to not have a parser error.
func (obj *FmtArgs) Run(ctx context.Context, data *cliUtil.Data) (bool, error) {

	Logf := func(format string, v ...interface{}) {
		// Don't block this globally...
		//if !data.Flags.Debug {
		//	return
		//}
		data.Flags.Logf("main: "+format, v...)
	}

	input := obj.Input
	if input == "" || input == "." {
		d, err := os.Getwd()
		if err != nil {
			return false, err
		}
		if !strings.HasSuffix(d, "/") {
			d += "/" // dirs must end with a slash!
		}
		input = d
	}
	isDir := strings.HasSuffix(input, "/") // did we ask for a dir?
	if !strings.HasPrefix(input, "/") {    // is the path absolute?
		d, err := os.Getwd()
		if err != nil {
			return false, err
		}

		input = filepath.Join(d, input) // removes any trailing slash :/
		if isDir && !strings.HasSuffix(input, "/") {
			input += "/" // add back the missing trailing slash
		}
	}

	// We can't import the parser from here without causing an import
	// cycle, so the lang frontend supplies a fully wired formatter to us
	// through the gapi registry instead.
	fn, exists := gapi.RegisteredGAPIs["lang"]
	if !exists {
		return true, fmt.Errorf("the lang frontend is not available")
	}
	provider, ok := fn().(format.FormatterProvider)
	if !ok {
		// programming error
		return true, fmt.Errorf("the lang frontend can not format")
	}
	formatter := provider.Formatter()
	formatter.Test = obj.Test
	formatter.Verbose = obj.Verbose
	formatter.Debug = data.Flags.Debug
	formatter.Logf = Logf
	formatter.Init()
	checkOK, err := formatter.FormatPath(ctx, input)
	if checkOK && err != nil {
		// programming error
		return true, fmt.Errorf("unexpected result")
	}
	if err != nil {
		return true, errwrap.Wrapf(err, "could not format input")
	}
	if checkOK { // nothing happened, everything already formatted correctly
		return true, nil
	}
	// here checkOK must be false...

	if !obj.Test {
		// we must have made changes, still we want to return 0
		return true, nil
	}

	// return non-zero to the caller, we aren't formatted correctly
	return true, fmt.Errorf("not formatted")
}

// Description returns a description string. Implementing this signature is part
// of the API for the cli library.
// XXX: Is this used by the go-arg cli library?
func (obj *FmtArgs) Description() string {
	return "format mcl code"
}

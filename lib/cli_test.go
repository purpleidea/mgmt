// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package lib

import (
	"flag"
	"io/ioutil"
	"strings"
	"testing"

	errwrap "github.com/pkg/errors"

	"github.com/urfave/cli"
)

// TestValidateCliPass tests if cli invocation of validation is able to pass on valid files
func TestValidateCliPass(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "pass.mcl")
	if err != nil {
		t.Fatal(errwrap.Wrapf(err, "can't create temp file"))
	}
	filePath := tmpFile.Name() // path to temp file
	defer tmpFile.Close()
	if _, err := tmpFile.Write([]byte("$x = 1")); err != nil {
		t.Fatal(errwrap.Wrapf(err, "can't write file"))
	}

	// create command line arguments for validating the file
	set := flag.NewFlagSet("test", 0)
	set.String("lang", filePath, "doc")
	ctx := cli.NewContext(nil, set, nil)

	// invoke validate subcommand with arguments to validate the file
	if err := validate(ctx); err != nil {
		t.Fatal(errwrap.Wrapf(err, "valid file should pass validation"))
	}
}

// TestValidateCliFail tests if cli invocation of validation is able to fail on invalid files
func TestValidateCliFail(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "fail.mcl")
	if err != nil {
		t.Fatal(errwrap.Wrapf(err, "can't create temp file"))
	}
	filePath := tmpFile.Name() // path to temp file
	defer tmpFile.Close()
	if _, err := tmpFile.Write([]byte("$x = 1; $x = 2")); err != nil {
		t.Fatal(errwrap.Wrapf(err, "can't write file"))
	}

	// create command line arguments for validating the file
	set := flag.NewFlagSet("test", 0)
	set.String("lang", filePath, "doc")
	ctx := cli.NewContext(nil, set, nil)

	// invoke validate subcommand with arguments to validate the file
	if err := validate(ctx); err == nil {
		t.Fatal(errwrap.Wrapf(err, "invalid file should _not_ pass validation"))
	}
}

// TestValidateError tests if validation fails on codes with different kind of invalid syntax
func TestValidateError(t *testing.T) {
	samples := []string{
		`file "/tmp/mgmt" { content => "test" }`, // syntaxerror, missing trailing comma
		"$x = 1; $x = 2",                         // scoperror, double assignment
		"$x = $x1 + 1",                           // unificationerror, $x1 does not exist
		`$y = ""; $x = $y + 1`,                   // unificationerror, types don't match
	}
	for _, s := range samples {
		t.Run("", func(st *testing.T) {
			sample := strings.NewReader(s)
			if err := validateCode(sample); err == nil {
				st.Fatal("example should _not_ pass validation")
			}
		})
	}
}

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
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	errwrap "github.com/pkg/errors"
	"github.com/purpleidea/mgmt/lang"
	"github.com/urfave/cli"
)

// validate handles the cli logic and reading code from a file/stdin for validation
func validate(c *cli.Context) error {
	filepath := c.String("lang")

	if len(filepath) == 0 {
		return fmt.Errorf("please provide path for file to validate")
	}

	var code io.Reader
	// allow to use - to read file from stdin
	if filepath == "-" {
		code = bufio.NewReader(os.Stdin)
	} else {
		filecontent, err := ioutil.ReadFile(filepath)
		if err != nil {
			return errwrap.Wrapf(err, "can't read code from file `%s`", filepath)
		}
		code = strings.NewReader(string(filecontent))
	}

	err := validateCode(code)

	if err != nil {
		// TODO: change format to some open generic error format supported by CI tools (line number etc)?
		if filepath != "-" {
			return errwrap.Wrapf(err, filepath)
		}
		return err
	}

	return err
}

// validateCode validates code from a reader
func validateCode(code io.Reader) error {
	// validate the code, suppressing normal compile log output
	obj := &lang.Lang{Input: code, Logf: func(format string, v ...interface{}) {}}
	err := obj.Validate()
	return err
}

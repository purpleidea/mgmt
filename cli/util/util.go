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

// Package util has some CLI related utility code.
package util

import (
	"strings"

	"github.com/purpleidea/mgmt/util/errwrap"
)

// Error is a constant error type that implements error.
type Error string

// Error fulfills the error interface of this type.
func (e Error) Error() string { return string(e) }

const (
	// MissingEquals means we probably hit the parsing bug.
	// XXX: see: https://github.com/alexflint/go-arg/issues/239
	MissingEquals = Error("missing equals sign for list element")
)

// CliParseError returns a consistent error if we have a CLI parsing issue.
func CliParseError(err error) error {
	return errwrap.Wrapf(err, "cli parse error")
}

// Flags are some constant flags which are used throughout the program.
// TODO: Unify this with Debug and Logf ?
type Flags struct {
	Debug   bool // add additional log messages
	Verbose bool // add extra log message output
}

// Data is a struct of values that we usually pass to the main CLI function.
type Data struct {
	Program string
	Version string
	Copying string
	Tagline string
	Flags   Flags
	Args    []string // os.Args usually
}

// SafeProgram returns the correct program string when given a buggy variant.
func SafeProgram(program string) string {
	// FIXME: in sub commands, the cli package appends a space and the sub
	// command name at the end. hack around this by only using the first bit
	// see: https://github.com/urfave/cli/issues/783 for more details...
	split := strings.Split(program, " ")
	program = split[0]
	//if program == "" {
	//	program = "<unknown>"
	//}
	return program
}

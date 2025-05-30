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

// Package errwrap contains some error helpers.
package errwrap

import (
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
)

// Wrapf adds a new error onto an existing chain of errors. If the new error to
// be added is nil, then the old error is returned unchanged.
func Wrapf(err error, format string, args ...interface{}) error {
	return errors.Wrapf(err, format, args...)
}

// Append can be used to safely append an error onto an existing one. If you
// pass in a nil error to append, the existing error will be returned unchanged.
// If the existing error is already nil, then the new error will be returned
// unchanged. This makes it easy to use Append as a safe `reterr += err`, when
// you don't know if either is nil or not.
func Append(reterr, err error) error {
	if reterr == nil { // keep it simple, pass it through
		return err // which might even be nil
	}
	if err == nil { // no error, so don't do anything
		return reterr
	}
	// both are real errors
	return multierror.Append(reterr, err)
}

// String returns a string representation of the error. In particular, if the
// error is nil, it returns an empty string instead of panicking.
func String(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

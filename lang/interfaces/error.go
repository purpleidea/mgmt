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

package interfaces

import (
	"errors"
	"fmt"

	"github.com/purpleidea/mgmt/util"
)

const (
	// ErrTypeCurrentlyUnknown is returned from the Type() call on Expr if
	// unification didn't run successfully and the type isn't obvious yet.
	// Note that it is perfectly legal to return any error, but this one can
	// be used instead of inventing your own.
	ErrTypeCurrentlyUnknown = util.Error("type is currently unknown")

	// ErrValueCurrentlyUnknown is returned from the Value() call on Expr if
	// we're speculating and we don't know a value statically. Note that it
	// is perfectly legal to return any error, but this one can be used
	// instead of inventing your own.
	ErrValueCurrentlyUnknown = util.Error("value is currently unknown")

	// ErrExpectedFileMissing is returned when a file that is used by an
	// import is missing. This might signal the downloader, or it might
	// signal a permanent error.
	ErrExpectedFileMissing = util.Error("file is currently missing")

	// ErrInterrupt is returned when a function can't immediately return a
	// value to the function engine because a graph change transaction needs
	// to run.
	ErrInterrupt = util.Error("function call interrupted")
)

// SentinelError wraps an error to mark it as "catchable". By default, any error
// returned from the Call method of a function is a permanent error, and it will
// shut down the function engine instantly. This is the right choice for
// programming errors and other truly unexpected situations. Some errors are an
// expected possibility however, such as a missing list index or map key in a
// lookup, or a division by zero. Functions which want those to be recoverable
// must wrap them with this type. The function engine will then propagate them
// downstream along the graph edges in place of a value, giving the except
// operator a chance to catch them and provide a fallback value. If nobody
// catches one, it is fatal when it arrives at a sink. Wrap errors with the
// Sentinelf helper function to return them conveniently.
type SentinelError struct {

	// Err is the error we are wrapping. It must not be nil.
	Err error
}

// Error implements the error interface for this special error type.
func (obj *SentinelError) Error() string { return obj.Err.Error() }

// Unwrap lets the standard errors package look inside of this error type.
func (obj *SentinelError) Unwrap() error { return obj.Err }

// Sentinelf works like fmt.Errorf, except that the returned error is marked as
// "catchable" for the function engine. Use it in the Call method of a function
// for any error which should be catchable by the except operator, instead of
// being an instant, permanent, fatal error.
func Sentinelf(format string, v ...interface{}) error {
	return &SentinelError{
		Err: fmt.Errorf(format, v...),
	}
}

// IsSentinelError returns true if the error is of type SentinelError.
func IsSentinelError(err error) bool {
	var sentinel *SentinelError
	return errors.As(err, &sentinel)
}

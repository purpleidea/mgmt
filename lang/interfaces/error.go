// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package interfaces

// Error is a constant error type that implements error.
type Error string

// Error fulfills the error interface of this type.
func (e Error) Error() string { return string(e) }

const (
	// ErrTypeCurrentlyUnknown is returned from the Type() call on Expr if
	// unification didn't run successfully and the type isn't obvious yet.
	ErrTypeCurrentlyUnknown = Error("type is currently unknown")

	// ErrExpectedFileMissing is returned when a file that is used by an
	// import is missing. This might signal the downloader, or it might
	// signal a permanent error.
	ErrExpectedFileMissing = Error("file is currently missing")
)

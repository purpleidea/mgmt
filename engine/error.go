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

package engine

// Error is a constant error type that implements error.
type Error string

// Error fulfills the error interface of this type.
func (e Error) Error() string { return string(e) }

const (
	// ErrWatchExit represents an exit from the Watch loop via chan closure.
	ErrWatchExit = Error("watch exit")

	// ErrSignalExit represents an exit from the Watch loop via exit signal.
	ErrSignalExit = Error("signal exit")
)

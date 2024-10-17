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

package util

import (
	"os"
	"syscall"
)

// TODO: Consider replacing this with: github.com/gofrs/flock if it's better.

// NewFlock creates a new flock. Details available in the struct documentation.
func NewFlock(path string) *Flock {
	return &Flock{
		Path: path,
	}
}

// Flock is a structure for building a lock file. This can be used to prevent a
// binary from running more than one copy at a time on a single machine.
type Flock struct {
	Path string

	file *os.File
}

// TryLock attempts to take the lock. If it errors, it means we couldn't build a
// lock today. If it returns (nil, nil) it means someone is already locked. If
// it returns a non-nil function, then you are now locked. Call that function to
// unlock. Note that it can error if it fails to unlock things.
func (obj *Flock) TryLock() (func() error, error) {
	var err error
	obj.file, err = os.Create(obj.Path)
	if err != nil {
		return nil, err // actual error, can't continue
	}

	// Exclusive lock in non-blocking mode, we add a shared lock.
	if err := syscall.Flock(int(obj.file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, nil // can't lock
	}

	// We're locked!
	return func() error {
		defer obj.file.Close()
		return syscall.Flock(int(obj.file.Fd()), syscall.LOCK_UN)
	}, nil
}

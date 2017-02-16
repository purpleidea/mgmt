// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package resources

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	errwrap "github.com/pkg/errors"
)

// Refresh returns the pending state of a notification. It should only be called
// in the CheckApply portion of a resource where a refresh should be acted upon.
func (obj *BaseRes) Refresh() bool {
	return obj.refresh
}

// SetRefresh sets the pending state of a notification. It should only be called
// by the mgmt engine.
func (obj *BaseRes) SetRefresh(b bool) {
	obj.refresh = b
}

// StatefulBool is an interface for storing a boolean flag in a permanent spot.
type StatefulBool interface {
	Get() (bool, error) // get value of token
	Set() error         // set token to true
	Del() error         // rm token if it exists
}

// DiskBool stores a boolean variable on disk for stateful access across runs.
// The absence of the path is treated as false. If the path contains a special
// value, then it is treated as true. All the other non-error cases are false.
type DiskBool struct {
	Path string // path to token
}

// str returns the string data which represents true (aka set).
func (obj *DiskBool) str() string {
	const TrueToken = "true"
	const newline = "\n"
	return TrueToken + newline
}

// Get returns if the boolean setting, if no error reading the value occurs.
func (obj *DiskBool) Get() (bool, error) {
	file, err := os.Open(obj.Path) // open a handle to read the file
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // no token means value is false
		}
		return false, errwrap.Wrapf(err, "could not read token")
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return false, errwrap.Wrapf(err, "could not read from file")
	}
	return strings.TrimSpace(string(data)) == strings.TrimSpace(obj.str()), nil
}

// Set stores the true boolean value, if no error setting the value occurs.
func (obj *DiskBool) Set() error {
	file, err := os.Create(obj.Path) // open a handle to create the file
	if err != nil {
		return errwrap.Wrapf(err, "can't create file")
	}
	defer file.Close()
	str := obj.str()
	if c, err := file.Write([]byte(str)); err != nil {
		return errwrap.Wrapf(err, "error writing to file")
	} else if l := len(str); c != l {
		return fmt.Errorf("wrote %d bytes instead of %d", c, l)
	}
	return file.Sync() // guarantee it!
}

// Del stores the false boolean value, if no error clearing the value occurs.
func (obj *DiskBool) Del() error {
	if err := os.Remove(obj.Path); err != nil { // remove the file
		if os.IsNotExist(err) {
			return nil // no file means this is already fine
		}
		return errwrap.Wrapf(err, "could not delete token")
	}
	return nil
}

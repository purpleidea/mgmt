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

package lib

import (
	"os"
	"strconv"
	"strings"
)

// raiseLimits is a helper to raise some of our limits to prevent us hitting the
// maximums too easily.
func raiseLimits(logf func(format string, v ...interface{})) (bool, error) {
	b1, err1 := raisePathValue("/proc/sys/fs/inotify/max_user_instances", 512, logf)
	b2, err2 := raisePathValue("/proc/sys/fs/inotify/max_user_watches", 242940, logf)
	b := true

	if err1 == nil {
		b = b && b1
	} else {
		logf("error raising limit: %v", err1)
	}
	if err2 == nil {
		b = b && b2
	} else {
		logf("error raising limit: %v", err2)
	}

	if err1 == nil && err2 == nil {
		return b, nil
	}
	if err1 == nil {
		return false, err2
	}
	return false, err1
}

func raisePathValue(path string, value int, logf func(format string, v ...interface{})) (bool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		// system or permissions error?
		return false, err
	}

	i, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return false, err
	}

	if i >= value { // if value is greater, then leave it alone =D
		return true, nil // did nothing
	}

	data := []byte(strconv.Itoa(value) + "\n")

	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, err
	}
	logf("raised limit of %s to %d", path, value)

	return false, nil
}

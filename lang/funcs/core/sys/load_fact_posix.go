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

//go:build !darwin

package coresys

import (
	"syscall"
)

const (
	// LoadScale factor scales the output from sysinfo to the correct float
	// value.
	LoadScale = 65536 // XXX: is this correct or should it be 65535?
)

// load returns the system load averages for the last minute, five minutes and
// fifteen minutes. Calling this more often than once every five seconds seems
// to be unnecessary, since the kernel only updates these values that often.
// TODO: is the kernel update interval configurable?
func load() (one, five, fifteen float64, err error) {
	var sysinfo syscall.Sysinfo_t
	if err = syscall.Sysinfo(&sysinfo); err != nil {
		return
	}
	one = float64(sysinfo.Loads[0]) / LoadScale
	five = float64(sysinfo.Loads[1]) / LoadScale
	fifteen = float64(sysinfo.Loads[2]) / LoadScale
	return
}

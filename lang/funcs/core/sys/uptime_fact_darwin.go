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

//go:build darwin

package coresys

import (
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func uptime() (int64, error) {
	// get time of boot struct
	rawBoottime, err := unix.SysctlRaw("kern.boottime")
	if err != nil {
		return 0, err
	}

	// make sure size is correct
	var boottime syscall.Timeval
	if len(rawBoottime) != int(unsafe.Sizeof(boottime)) {
		return 0, fmt.Errorf("invalid boottime encountered while calculating uptime")
	}

	// uptime is difference between current time and boot time
	boottime = *(*syscall.Timeval)(unsafe.Pointer(&rawBoottime[0]))
	return time.Now().Unix() - boottime.Sec, nil
}

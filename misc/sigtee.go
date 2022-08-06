// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

// This program works exactly like the `tee` program, if it was invoked with an
// implicit --ignore-interrupts argument, and if it also had an --ignore-quit
// argument. These are needed so that it doesn't exit prematurely when called as
// a receiving member of a shell pipeline. This is needed so that a ^C or ^\ can
// cause the sending process to shutdown and relay its data into the tee. Sadly,
// the venerable `tee` program can't currently ignore the QUIT signal.
package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// TODO: Add better argv parsing and implement explicit flags for
	// --ignore-interrupts argument, and --ignore-quit so that it's cleaner.
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: <STDIN> | %s <FILE>\n", os.Args[0])
		os.Exit(1)
		return
	}
	filename := os.Args[1]

	// Make sure we ignore ^C and ^\ when run in a shell pipe.
	signal.Ignore(os.Interrupt, syscall.SIGQUIT) // TODO: add os.Kill ?

	f, err := os.Create(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "can't write to: %s\n", filename)
		os.Exit(1)
		return
	}
	defer f.Close()

	writer := io.MultiWriter(os.Stdout, f) // tee !

	_, err = io.Copy(writer, os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "copy error: %+v\n", err)
		os.Exit(1)
		return
	}
}

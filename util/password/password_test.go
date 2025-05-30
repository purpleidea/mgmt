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

//go:build !root

package password

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func ExampleReadPasswordCtx() {
	// Put this in a main function and it will not have the ioctl error!
	fmt.Println("hello")
	defer fmt.Println("exiting...")

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// If we exit without getting a chance to reset the terminal, it might
	// be borked!
	ch := make(chan os.Signal, 1+1) // must have buffer for max number of signals
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)
	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ch:
			cancel()
		case <-ctx.Done():
		}
	}()

	password, err := ReadPasswordCtx(ctx)
	if err != nil {
		fmt.Printf("error: %+v\n", err)
		return
	}

	fmt.Printf("password is: %s\n", string(password))

	// Output: hello
	// error: inappropriate ioctl for device
	// exiting...
}

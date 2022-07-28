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

package coreos

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	funcs.ModuleRegister(ModuleName, "system", func() interfaces.Func { return &SystemFunc{} })
}

// SystemFunc runs a string as a shell command, then produces each line from
// stdout. If the input string changes, then the commands are executed one after
// the other and the concatenation of their outputs is produced line by line.
//
// Note that in the likely case in which the process emits several lines one
// after the other, the downstream resources might not run for every line unless
// the "Meta:realize" metaparam is set to true.
type SystemFunc struct {
	init *interfaces.Init

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *SystemFunc) ArgGen(index int) (string, error) {
	seq := []string{"shell_command"}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *SystemFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *SystemFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Sig:  types.NewType("func(shell_command str) str"),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *SystemFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *SystemFunc) Stream() error {
	// Close the output chan to signal that no more values are coming.
	defer close(obj.init.Output)

	// A channel which closes when the current process exits, on its own
	// or due to cancel(). The channel is only closed once all the pending
	// stdout and stderr lines have been processed.
	//
	// The channel starts closed because no process is running yet. A new
	// channel is created each time a new process is started. We never run
	// more than one process at a time.
	processedChan := make(chan struct{})
	close(processedChan)

	// Wait for the current process to exit, if any.
	defer func() {
		<-processedChan
	}()

	// Kill the current process, if any. A new cancel function is created
	// each time a new process is started.
	var ctx context.Context
	var cancel context.CancelFunc
	cancel = func() {}
	defer cancel()

	for {
		select {
		case input, more := <-obj.init.Input:
			if !more {
				// Wait until the current process exits and all of its
				// stdout is sent downstream.
				select {
				case <-processedChan:
					return nil
				case <-obj.closeChan:
					return nil
				}
			}
			shellCommand := input.Struct()["shell_command"].Str()

			// Kill the previous command, if any.
			cancel()
			<-processedChan

			// Run the command, connecting it to ctx so we can kill
			// it if needed, and to two Readers so we can read its
			// stdout and stderr.
			ctx, cancel = context.WithCancel(context.Background())
			cmd := exec.CommandContext(ctx, "sh", "-c", shellCommand)
			stdoutReader, err := cmd.StdoutPipe()
			if err != nil {
				return err
			}
			stderrReader, err := cmd.StderrPipe()
			if err != nil {
				return err
			}
			if err = cmd.Start(); err != nil {
				return err
			}

			// We will now start several goroutines:
			// 1. To process stdout
			// 2. To process stderr
			// 3. To wait for (1) and (2) to terminate and close processedChan
			//
			// This WaitGroup is used by (3) to wait for (1) and (2).
			wg := &sync.WaitGroup{}

			// Emit one value downstream for each line from stdout.
			// Terminates when the process exits, on its own or due
			// to cancel().
			wg.Add(1)
			go func() {
				defer wg.Done()

				stdoutScanner := bufio.NewScanner(stdoutReader)
				for stdoutScanner.Scan() {
					outputValue := &types.StrValue{V: stdoutScanner.Text()}
					obj.init.Output <- outputValue
				}
			}()

			// Log the lines from stderr, to help the user debug.
			// Terminates when the process exits, on its own or
			// due to cancel().
			wg.Add(1)
			go func() {
				defer wg.Done()

				stderrScanner := bufio.NewScanner(stderrReader)
				for stderrScanner.Scan() {
					obj.init.Logf("system: \"%v\": stderr: %v\n", shellCommand, stderrScanner.Text())
				}
			}()

			// Closes processedChan after the previous two
			// goroutines terminate. Thus, this goroutine also
			// terminates when the process exits, on its own or due
			// to cancel().
			processedChan = make(chan struct{})
			go func() {
				wg.Wait()
				close(processedChan)
			}()
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *SystemFunc) Close() error {
	close(obj.closeChan)
	return nil
}

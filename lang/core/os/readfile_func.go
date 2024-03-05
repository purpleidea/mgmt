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

package coreos

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ReadFileFuncName is the name this function is registered as.
	ReadFileFuncName = "readfile"

	// arg names...
	readFileArgNameFilename = "filename"
)

func init() {
	funcs.ModuleRegister(ModuleName, ReadFileFuncName, func() interfaces.Func { return &ReadFileFunc{} }) // must register the func and name
}

// ReadFileFunc is a function that reads the full contents from a local file. If
// the file contents change or the file path changes, a new string will be sent.
// Please note that this is different from the readfile function in the deploy
// package.
type ReadFileFunc struct {
	init *interfaces.Init
	last types.Value // last value received to use for diff

	filename   *string // the active filename
	recWatcher *recwatch.RecWatcher
	events     chan error // internal events
	wg         *sync.WaitGroup

	result *string // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ReadFileFunc) String() string {
	return ReadFileFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ReadFileFunc) ArgGen(index int) (string, error) {
	seq := []string{readFileArgNameFilename}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ReadFileFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ReadFileFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the file contents can change
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) str", readFileArgNameFilename)),
	}
}

// Init runs some startup code for this function.
func (obj *ReadFileFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.events = make(chan error)
	obj.wg = &sync.WaitGroup{}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ReadFileFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	defer close(obj.events)      // clean up for fun
	defer obj.wg.Wait()
	defer func() {
		if obj.recWatcher != nil {
			obj.recWatcher.Close() // close previous watcher
			obj.wg.Wait()
		}
	}()
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			filename := input.Struct()[readFileArgNameFilename].Str()
			// TODO: add validation for absolute path?
			// TODO: add check for empty string
			if obj.filename != nil && *obj.filename == filename {
				continue // nothing changed
			}
			obj.filename = &filename

			if obj.recWatcher != nil {
				obj.recWatcher.Close() // close previous watcher
				obj.wg.Wait()
			}
			// create new watcher
			obj.recWatcher = &recwatch.RecWatcher{
				Path:    *obj.filename,
				Recurse: false,
				Flags: recwatch.Flags{
					// TODO: add Logf
					Debug: obj.init.Debug,
				},
			}
			if err := obj.recWatcher.Init(); err != nil {
				obj.recWatcher = nil
				// TODO: should we ignore the error and send ""?
				return errwrap.Wrapf(err, "could not watch file")
			}

			// FIXME: instead of sending one event here, the recwatch
			// library should send one initial event at startup...
			startup := make(chan struct{})
			close(startup)

			// watch recwatch events in a proxy goroutine, since
			// changing the recwatch object would panic the main
			// select when it's nil...
			obj.wg.Add(1)
			go func() {
				defer obj.wg.Done()
				for {
					var err error
					select {
					case <-startup:
						startup = nil
						// send an initial event

					case event, ok := <-obj.recWatcher.Events():
						if !ok {
							return // file watcher shut down
						}
						if err = event.Error; err != nil {
							err = errwrap.Wrapf(err, "error event received")
						}
					}

					select {
					case obj.events <- err:
						// send event...

					case <-ctx.Done():
						// don't block here on shutdown
						return
					}
					//err = nil // reset
				}
			}()
			continue // wait for an actual event or we'd send empty!

		case err, ok := <-obj.events:
			if !ok {
				return fmt.Errorf("no more events")
			}
			if err != nil {
				return errwrap.Wrapf(err, "error event received")
			}

			if obj.last == nil {
				continue // still waiting for input values
			}

			// read file...
			content, err := os.ReadFile(*obj.filename)
			if err != nil {
				return errwrap.Wrapf(err, "error reading file")
			}
			result := string(content) // convert to string

			if obj.result != nil && *obj.result == result {
				continue // result didn't change
			}
			obj.result = &result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: *obj.result,
		}:
		case <-ctx.Done():
			return nil
		}
	}
}

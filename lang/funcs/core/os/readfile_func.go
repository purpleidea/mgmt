// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	funcs.ModuleRegister(ModuleName, "readfile", func() interfaces.Func { return &ReadFileFunc{} }) // must register the func and name
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

	closeChan chan struct{}
}

// ArgGen returns the Nth arg name for this function.
func (obj *ReadFileFunc) ArgGen(index int) (string, error) {
	seq := []string{"filename"}
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
		Sig:  types.NewType("func(filename str) str"),
	}
}

// Init runs some startup code for this function.
func (obj *ReadFileFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.events = make(chan error)
	obj.wg = &sync.WaitGroup{}
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ReadFileFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
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

			filename := input.Struct()["filename"].Str()
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

					case <-obj.closeChan:
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
			content, err := ioutil.ReadFile(*obj.filename)
			if err != nil {
				return errwrap.Wrapf(err, "error reading file")
			}
			result := string(content) // convert to string

			if obj.result != nil && *obj.result == result {
				continue // result didn't change
			}
			obj.result = &result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: *obj.result,
		}:
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *ReadFileFunc) Close() error {
	close(obj.closeChan)
	obj.wg.Wait()     // block so we don't exit by the closure of obj.events
	close(obj.events) // clean up for fun
	return nil
}

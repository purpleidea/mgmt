// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package core // TODO: should this be in its own individual package?

import (
	"io/ioutil"
	"os"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/recwatch"

	errwrap "github.com/pkg/errors"
)

func init() {
	funcs.Register("file", func() interfaces.Func { return &FileFunc{} }) // must register the func and name
}

// FileFunc returns the content of a file on the filesystem
type FileFunc struct {
	init *interfaces.Init

	filePath string

	recWatcher *recwatch.RecWatcher

	result string // last calculated output

	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *FileFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *FileFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig:  types.NewType("func(path str) str"),
	}
}

// Init runs some startup code for this function.
func (obj *FileFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *FileFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes

	var update bool
	var watcherEvents chan recwatch.Event

	for {
		select {
		// respond if function inputs change
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}

			filePath := input.Struct()["path"].Str()
			if obj.filePath == filePath {
				continue
			}

			update = true
			obj.filePath = filePath

			// initialize watcher on (new) file path
			var err error
			obj.recWatcher, err = recwatch.NewRecWatcher(obj.filePath, true)
			if err != nil {
				return errwrap.Wrapf(err, "failed to watch file")
			}

			// enable the watcher channel
			watcherEvents = obj.recWatcher.Events()

		// add watcher for file changes
		// inititially this channel in nil as the watch channel is added when the file path is received
		case event, ok := <-watcherEvents:
			if !ok { // channel shutdown
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown %s watcher error", obj)
			}
			update = true
		case <-obj.closeChan:
			if obj.recWatcher != nil {
				obj.recWatcher.Close()
			}
			return nil
		}

		if update {
			update = false

			if _, err := os.Stat(obj.filePath); os.IsNotExist(err) {
				// if file does not exist (or gets deleted), return empty
				obj.result = ""
			} else {
				// read file content
				data, err := ioutil.ReadFile(obj.filePath)
				if err != nil {
					return errwrap.Wrapf(err, "failed to read file")
				}
				// TODO: is there a way to not store the content in memory?
				obj.result = string(data)
			}
		}

		select {
		case obj.init.Output <- &types.StrValue{
			V: obj.result,
		}:
		case <-obj.closeChan:
			if obj.recWatcher != nil {
				obj.recWatcher.Close()
			}
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *FileFunc) Close() error {
	close(obj.closeChan)
	return nil
}

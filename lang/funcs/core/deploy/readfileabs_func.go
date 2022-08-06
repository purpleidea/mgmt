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

package coredeploy

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	funcs.ModuleRegister(ModuleName, "readfileabs", func() interfaces.Func { return &ReadFileAbsFunc{} }) // must register the func and name
}

// ReadFileAbsFunc is a function that reads the full contents from a file in our
// deploy. The file contents can only change with a new deploy, so this is
// static. In particular, this takes an absolute path relative to the root
// deploy. In general, you should use `deploy.readfile` instead. Please note
// that this is different from the readfile function in the os package.
type ReadFileAbsFunc struct {
	init *interfaces.Init
	data *interfaces.FuncData
	last types.Value // last value received to use for diff

	filename *string // the active filename
	result   *string // last calculated output

	closeChan chan struct{}
}

// SetData is used by the language to pass our function some code-level context.
func (obj *ReadFileAbsFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
}

// ArgGen returns the Nth arg name for this function.
func (obj *ReadFileAbsFunc) ArgGen(index int) (string, error) {
	seq := []string{"filename"}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ReadFileAbsFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ReadFileAbsFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the file contents can change
		Memo: false,
		Sig:  types.NewType("func(filename str) str"),
	}
}

// Init runs some startup code for this function.
func (obj *ReadFileAbsFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ReadFileAbsFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
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

			fs, err := obj.init.World.Fs(obj.data.FsURI) // open the remote file system
			if err != nil {
				return errwrap.Wrapf(err, "can't load code from file system `%s`", obj.data.FsURI)
			}
			content, err := fs.ReadFile(*obj.filename) // open the remote file system
			// We could use it directly, but it feels like less correct.
			//content, err := obj.data.Fs.ReadFile(*obj.filename) // open the remote file system
			if err != nil {
				return errwrap.Wrapf(err, "can't read file `%s`", *obj.filename)
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
func (obj *ReadFileAbsFunc) Close() error {
	close(obj.closeChan)
	return nil
}

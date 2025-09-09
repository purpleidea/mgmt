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

package coredeploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
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

var _ interfaces.DataFunc = &ReadFileFunc{}

// ReadFileFunc is a function that reads the full contents from a file in our
// deploy. The file contents can only change with a new deploy, so this is
// static. Please note that this is different from the readfile function in the
// os package.
type ReadFileFunc struct {
	init *interfaces.Init
	data *interfaces.FuncData
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ReadFileFunc) String() string {
	return ReadFileFuncName
}

// SetData is used by the language to pass our function some code-level context.
func (obj *ReadFileFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
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
		Fast: false,
		Spec: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) str", readFileArgNameFilename)),
	}
}

// Init runs some startup code for this function.
func (obj *ReadFileFunc) Init(init *interfaces.Init) error {
	obj.init = init
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Copy is implemented so that the obj.built value is not lost if we copy this
// function.
func (obj *ReadFileFunc) Copy() interfaces.Func {
	return &ReadFileFunc{
		init: obj.init, // likely gets overwritten anyways
		data: obj.data, // needed because we don't call SetData twice
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ReadFileFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	filename := args[0].Str()

	if obj.data == nil {
		return nil, funcs.ErrCantSpeculate
	}
	p := strings.TrimSuffix(obj.data.Base, "/")
	if p == obj.data.Base { // didn't trim, so we fail
		// programming error
		return nil, fmt.Errorf("no trailing slash on Base, got: `%s`", p)
	}
	path := p

	if !strings.HasPrefix(filename, "/") {
		return nil, fmt.Errorf("filename was not absolute, got: `%s`", filename)
		//path += "/" // be forgiving ?
	}
	path += filename

	if obj.init == nil || obj.data == nil {
		return nil, funcs.ErrCantSpeculate
	}
	fs, err := obj.init.World.Fs(obj.data.FsURI) // open the remote file system
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't load code from file system `%s`", obj.data.FsURI)
	}
	// this is relative to the module dir the func is in!
	content, err := fs.ReadFile(path) // open the remote file system
	// We could use it directly, but it feels like less correct.
	//content, err := obj.data.Fs.ReadFile(path) // open the remote file system
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read file `%s` (%s)", filename, path)
	}

	return &types.StrValue{
		V: string(content), // convert to string
	}, nil
}

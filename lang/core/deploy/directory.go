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
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DirectoryDeployPrefix is the prefix we prepend to any directory path
	// so that if the local.VarDir API is used elsewhere, it doesn't
	// conflict with what we're doing here.
	DirectoryDeployPrefix = "directory/"

	// DirectoryFuncName is the name this function is registered as.
	DirectoryFuncName = "directory"

	// arg names...
	directoryArgNameDirectory = "directory"
)

func init() {
	funcs.ModuleRegister(ModuleName, DirectoryFuncName, func() interfaces.Func { return &DirectoryFunc{} }) // must register the func and name
}

var _ interfaces.DataFunc = &DirectoryFunc{}

// DirectoryFunc is a function that reads the full contents of a directory from
// our deploy. It puts them into a private temporary directory and returns that
// path. The returned path will be an absolute directory path. The directory
// contents can only change with a new deploy, so this is static. This function
// effectively causes a double copy of the file contents, so only use this as a
// last resort.
//
// XXX: This function leaves leftover files from previous deploys, so be
// cautious at least until there is a better cleanup story here.
type DirectoryFunc struct {
	interfaces.Textarea

	init *interfaces.Init
	data *interfaces.FuncData
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *DirectoryFunc) String() string {
	return DirectoryFuncName
}

// SetData is used by the language to pass our function some code-level context.
func (obj *DirectoryFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
}

// ArgGen returns the Nth arg name for this function.
func (obj *DirectoryFunc) ArgGen(index int) (string, error) {
	seq := []string{directoryArgNameDirectory}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *DirectoryFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *DirectoryFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the file contents can change
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) str", directoryArgNameDirectory)),
	}
}

// Init runs some startup code for this function.
func (obj *DirectoryFunc) Init(init *interfaces.Init) error {
	obj.init = init
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Copy is implemented so that the obj.built value is not lost if we copy this
// function.
func (obj *DirectoryFunc) Copy() interfaces.Func {
	return &DirectoryFunc{
		Textarea: obj.Textarea,

		init: obj.init, // likely gets overwritten anyways
		data: obj.data, // needed because we don't call SetData twice
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *DirectoryFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	directory := args[0].Str()

	if obj.data == nil {
		return nil, funcs.ErrCantSpeculate
	}
	p := strings.TrimSuffix(obj.data.Base, "/")
	if p == obj.data.Base { // didn't trim, so we fail
		// programming error
		return nil, fmt.Errorf("no trailing slash on Base, got: `%s`", p)
	}

	if !strings.HasPrefix(directory, "/") {
		return nil, fmt.Errorf("directory was not absolute, got: `%s`", directory)
		//p += "/" // be forgiving ?
	}
	if !strings.HasSuffix(directory, "/") {
		return nil, fmt.Errorf("directory path must be a dir")
	}
	p += directory

	if obj.init == nil || obj.data == nil {
		return nil, funcs.ErrCantSpeculate
	}
	fs, err := obj.init.World.Fs(ctx, obj.data.FsURI) // open the remote file system
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't load data from file system `%s`", obj.data.FsURI)
	}
	// reldir could be something unique to this input arg, but might as well
	// let them overlap since this make this all a copy of the deploy dirs.
	// XXX: If we ever get involved with event stuff in this folder, it may
	// get messy, so look into that if we ever get there.
	reldir := p

	if !strings.HasPrefix(reldir, "/") {
		return nil, fmt.Errorf("path must be absolute")
	}
	if !strings.HasSuffix(reldir, "/") {
		return nil, fmt.Errorf("path must be a dir")
	}
	// TODO: clean this so we don't get `///` or similar garbage?
	if reldir == "//" {
		return nil, fmt.Errorf("path can't be two slashes")
	}
	// NOTE: The above checks ensure we don't get "//" as input!

	// XXX: do we want to make the dest dir a hash of the input path?
	prefix := fmt.Sprintf("%s/", path.Join(DirectoryDeployPrefix, reldir))

	result, err := obj.init.Local.VarDir(ctx, prefix)
	if err != nil {
		return nil, err
	}

	// Sync the deploy dir contents into our local var dir, `rsync --delete`
	// style. Unnecessary churn would cause unnecessary events!
	// XXX: We need some sort of long-term cleanup mechanism if deploys
	// constantly change and leave behind old files.
	// XXX: pull this sync function into a lib and verify it is correct
	var sync func(srcDir, dstDir string) error
	sync = func(srcDir, dstDir string) error {
		srcEntries, err := fs.ReadDir(srcDir)
		if err != nil {
			return err
		}
		keep := make(map[string]struct{}, len(srcEntries))
		for _, e := range srcEntries {
			keep[e.Name()] = struct{}{}
		}
		dstEntries, err := os.ReadDir(dstDir)
		if err != nil {
			return err
		}
		for _, e := range dstEntries {
			if _, ok := keep[e.Name()]; ok {
				continue
			}
			if err := os.RemoveAll(path.Join(dstDir, e.Name())); err != nil {
				return err
			}
		}

		for _, e := range srcEntries {
			srcChild := path.Join(srcDir, e.Name())
			dstChild := path.Join(dstDir, e.Name())
			dstInfo, _ := os.Stat(dstChild)

			if e.IsDir() {
				if dstInfo != nil && !dstInfo.IsDir() {
					if err := os.Remove(dstChild); err != nil {
						return err
					}
				}
				if err := os.MkdirAll(dstChild, e.Mode()); err != nil {
					return err
				}
				if err := sync(srcChild, dstChild); err != nil {
					return err
				}
				continue
			}

			if dstInfo != nil && dstInfo.IsDir() {
				if err := os.RemoveAll(dstChild); err != nil {
					return err
				}
				dstInfo = nil
			}
			data, err := fs.ReadFile(srcChild)
			if err != nil {
				return err
			}
			if dstInfo != nil {
				if existing, err := os.ReadFile(dstChild); err == nil && bytes.Equal(existing, data) {
					continue
				}
			}
			if err := os.WriteFile(dstChild, data, e.Mode()); err != nil {
				return err
			}
		}
		return nil
	}
	if err := sync(p, result); err != nil {
		return nil, errwrap.Wrapf(err, "can't sync dir `%s` (%s)", directory, p)
	}

	return &types.StrValue{
		V: result,
	}, nil
}

// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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
	"context"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// AbsPathFuncName is the name this function is registered as.
	AbsPathFuncName = "abspath"

	// arg names...
	absPathArgNamePath = "path"
)

func init() {
	funcs.ModuleRegister(ModuleName, AbsPathFuncName, func() interfaces.Func { return &AbsPathFunc{} }) // must register the func and name
}

// AbsPathFunc is a function that returns the absolute, full path in the deploy
// from an input path that is relative to the calling file. If you pass it an
// empty string, you'll just get the absolute deploy directory path that you're
// in.
type AbsPathFunc struct {
	init *interfaces.Init
	data *interfaces.FuncData
	last types.Value // last value received to use for diff

	path   *string // the active path
	result *string // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *AbsPathFunc) String() string {
	return AbsPathFuncName
}

// SetData is used by the language to pass our function some code-level context.
func (obj *AbsPathFunc) SetData(data *interfaces.FuncData) {
	obj.data = data
}

// ArgGen returns the Nth arg name for this function.
func (obj *AbsPathFunc) ArgGen(index int) (string, error) {
	seq := []string{absPathArgNamePath}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *AbsPathFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *AbsPathFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the file contents can change
		Memo: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) str", absPathArgNamePath)),
	}
}

// Init runs some startup code for this function.
func (obj *AbsPathFunc) Init(init *interfaces.Init) error {
	obj.init = init
	if obj.data == nil {
		// programming error
		return fmt.Errorf("missing function data")
	}
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *AbsPathFunc) Stream(ctx context.Context) error {
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

			path := input.Struct()[absPathArgNamePath].Str()
			// TODO: add validation for absolute path?
			if obj.path != nil && *obj.path == path {
				continue // nothing changed
			}
			obj.path = &path

			p := strings.TrimSuffix(obj.data.Base, "/")
			if p == obj.data.Base { // didn't trim, so we fail
				// programming error
				return fmt.Errorf("no trailing slash on Base, got: `%s`", p)
			}
			result := p

			if *obj.path == "" {
				result += "/" // add the above trailing slash back
			} else if !strings.HasPrefix(*obj.path, "/") {
				return fmt.Errorf("path was not absolute, got: `%s`", *obj.path)
				//result += "/" // be forgiving ?
			}
			result += *obj.path

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

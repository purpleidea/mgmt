// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package structs

import (
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// ConstFunc is a function that returns the constant value passed to Value.
type ConstFunc struct {
	Value types.Value

	init      *interfaces.Init
	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly.
func (obj *ConstFunc) Validate() error {
	if obj.Value == nil {
		return fmt.Errorf("must specify `Value` input")
	}
	return nil
}

// Info returns some static info about itself.
func (obj *ConstFunc) Info() *interfaces.Info {
	var typ *types.Type
	if obj.Value != nil { // don't panic if called speculatively
		if t := obj.Value.Type(); t != nil {
			typ = types.NewType(fmt.Sprintf("func() %s", t.String()))
		}
	}
	return &interfaces.Info{
		Pure: true,
		Memo: false, // TODO: ???
		Sig:  typ,
		Err:  obj.Validate(), // XXX: implement this and check .Err in engine!
	}
}

// Init runs some startup code for this const.
func (obj *ConstFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the single value that this const has, and then closes.
func (obj *ConstFunc) Stream() error {
	select {
	case obj.init.Output <- obj.Value:
		// pass
	case <-obj.closeChan:
		return nil
	}
	close(obj.init.Output) // signal that we're done sending
	return nil
}

// Close runs some shutdown code for this const and turns off the stream.
func (obj *ConstFunc) Close() error {
	close(obj.closeChan)
	return nil
}

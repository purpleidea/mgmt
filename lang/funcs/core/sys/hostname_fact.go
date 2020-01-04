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

package coresys

import (
	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	facts.ModuleRegister(ModuleName, "hostname", func() facts.Fact { return &HostnameFact{} }) // must register the fact and name
}

// HostnameFact is a function that returns the hostname.
// TODO: support hostnames that change in the future.
type HostnameFact struct {
	init      *facts.Init
	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *HostnameFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *HostnameFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("str"),
	}
}

// Init runs some startup code for this fact.
func (obj *HostnameFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the single value that this fact has, and then closes.
func (obj *HostnameFact) Stream() error {
	select {
	case obj.init.Output <- &types.StrValue{
		V: obj.init.Hostname,
	}:
		// pass
	case <-obj.closeChan:
		return nil
	}
	close(obj.init.Output) // signal that we're done sending
	return nil
}

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *HostnameFact) Close() error {
	close(obj.closeChan)
	return nil
}

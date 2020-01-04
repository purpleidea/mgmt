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

package coreexample

import (
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	facts.ModuleRegister(ModuleName, "flipflop", func() facts.Fact { return &FlipFlopFact{} }) // must register the fact and name
}

// FlipFlopFact is a fact which flips a bool repeatedly. This is an example fact
// and is not meant for serious computing. This would be better served by a flip
// function which you could specify an interval for.
type FlipFlopFact struct {
	init      *facts.Init
	value     bool
	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *FlipFlopFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *FlipFlopFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("bool"),
	}
}

// Init runs some startup code for this fact.
func (obj *FlipFlopFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *FlipFlopFact) Stream() error {
	defer close(obj.init.Output) // always signal when we're done
	// TODO: don't hard code 5 sec interval
	ticker := time.NewTicker(time.Duration(5) * time.Second)

	// streams must generate an initial event on startup
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!
	defer ticker.Stop()
	for {
		select {
		case <-startChan: // kick the loop once at start
			startChan = nil // disable
		case <-ticker.C: // received the timer event
			// pass
		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- &types.BoolValue{ // flip
			V: obj.value,
		}:
		case <-obj.closeChan:
			return nil
		}

		obj.value = !obj.value // flip it
	}
}

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *FlipFlopFact) Close() error {
	close(obj.closeChan)
	return nil
}

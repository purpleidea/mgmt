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

package coresys

import (
	"time"

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	facts.ModuleRegister(ModuleName, "uptime", func() facts.Fact { return &UptimeFact{} })
}

// UptimeFact is a fact which returns the current uptime of your system.
type UptimeFact struct {
	init      *facts.Init
	closeChan chan struct{}
}

// Info returns some static info about itself.
func (obj *UptimeFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.TypeInt,
	}
}

// Init runs some startup code for this fact.
func (obj *UptimeFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *UptimeFact) Stream() error {
	defer close(obj.init.Output)
	ticker := time.NewTicker(time.Duration(1) * time.Second)

	startChan := make(chan struct{})
	close(startChan)
	defer ticker.Stop()
	for {
		select {
		case <-startChan:
			startChan = nil
		case <-ticker.C:
			// send
		case <-obj.closeChan:
			return nil
		}

		uptime, err := uptime()
		if err != nil {
			return errwrap.Wrapf(err, "could not read uptime value")
		}

		select {
		case obj.init.Output <- &types.IntValue{V: uptime}:
			// send
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *UptimeFact) Close() error {
	close(obj.closeChan)
	return nil
}

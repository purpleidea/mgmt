// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

const (
	loadSignature = "struct{x1 float; x5 float; x15 float}"
)

func init() {
	facts.ModuleRegister(ModuleName, "load", func() facts.Fact { return &LoadFact{} }) // must register the fact and name
}

// LoadFact is a fact which returns the current system load.
type LoadFact struct {
	init      *facts.Init
	closeChan chan struct{}
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *LoadFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *LoadFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType(loadSignature),
	}
}

// Init runs some startup code for this fact.
func (obj *LoadFact) Init(init *facts.Init) error {
	obj.init = init
	obj.closeChan = make(chan struct{})
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *LoadFact) Stream() error {
	defer close(obj.init.Output) // always signal when we're done

	// it seems the different values only update once every 5
	// seconds, so that's as often as we need to refresh this!
	// TODO: lookup this value if it's something configurable
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

		x1, x5, x15, err := load()
		if err != nil {
			return errwrap.Wrapf(err, "could not read load values")
		}

		st := types.NewStruct(types.NewType(loadSignature))
		for k, v := range map[string]float64{"x1": x1, "x5": x5, "x15": x15} {
			if err := st.Set(k, &types.FloatValue{V: v}); err != nil {
				return errwrap.Wrapf(err, "struct could not set key: `%s`", k)
			}
		}

		select {
		case obj.init.Output <- st:
			// send
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this fact and turns off the stream.
func (obj *LoadFact) Close() error {
	close(obj.closeChan)
	return nil
}

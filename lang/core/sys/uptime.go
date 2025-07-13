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

package coresys

import (
	"context"
	"fmt"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// UptimeFuncName is the name this fact is registered as. It's still a
	// Func Name because this is the name space the fact is actually using.
	UptimeFuncName = "uptime"
)

func init() {
	funcs.ModuleRegister(ModuleName, UptimeFuncName, func() interfaces.Func { return &Uptime{} })
}

// Uptime is a fact which returns the current uptime of your system.
type Uptime struct {
	init *interfaces.Init
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *Uptime) String() string {
	return UptimeFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *Uptime) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *Uptime) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() int"),
	}
}

// Init runs some startup code for this fact.
func (obj *Uptime) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *Uptime) Stream(ctx context.Context) error {
	defer close(obj.init.Output)

	// We always wait for our initial event to start.
	select {
	case _, ok := <-obj.init.Input:
		if ok {
			return fmt.Errorf("unexpected input")
		}
		obj.init.Input = nil

	case <-ctx.Done():
		return nil
	}

	ticker := time.NewTicker(time.Duration(1) * time.Second)
	defer ticker.Stop()

	// streams must generate an initial event on startup
	// even though ticker will send one, we want to be faster to first event
	startChan := make(chan struct{}) // start signal
	close(startChan)                 // kick it off!

	for {
		select {
		case <-startChan:
			startChan = nil // disable

		case <-ticker.C:
			// send

		case <-ctx.Done():
			return nil
		}

		result, err := obj.Call(ctx, nil)
		if err != nil {
			return err
		}

		select {
		case obj.init.Output <- result:

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *Uptime) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	uptime, err := uptime() // TODO: add ctx?
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not read uptime value")
	}

	return &types.IntValue{
		V: uptime,
	}, nil
}

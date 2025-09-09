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

package coredatetime

import (
	"context"
	"time"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// NowFuncName is the name this fact is registered as. It's still a Func
	// Name because this is the name space the fact is actually using.
	NowFuncName = "now"
)

func init() {
	funcs.ModuleRegister(ModuleName, NowFuncName, func() interfaces.Func { return &Now{} }) // must register the fact and name
}

// Now is a fact which returns the current date and time.
type Now struct {
	init *interfaces.Init
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *Now) String() string {
	return NowFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *Now) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *Now) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() int"),
	}
}

// Init runs some startup code for this fact.
func (obj *Now) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream starts a mainloop and runs Event when it's time to Call() again.
func (obj *Now) Stream(ctx context.Context) error {
	// XXX: this might be an interesting fact to write because:
	// 1) will the sleeps from the ticker be in sync with the second ticker?
	// 2) if we care about a less precise interval (eg: minute changes) can
	// we set this up so it doesn't tick as often? -- Yes (make this a function or create a limit function to wrap this)
	// 3) is it best to have a delta timer that wakes up before it's needed
	// and calculates how much longer to sleep for?
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

		case <-ticker.C: // received the timer event
			// pass

		case <-ctx.Done():
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *Now) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.IntValue{ // seconds since 1970...
		V: time.Now().Unix(), // .UTC() not necessary
	}, nil
}

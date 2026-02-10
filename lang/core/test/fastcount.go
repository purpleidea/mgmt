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

package coretest

import (
	"context"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// FastCountFuncName is the name this fact is registered as. It's still
	// a Func Name because this is the name space the fact is actually
	// using.
	FastCountFuncName = "fastcount"
)

func init() {
	funcs.ModuleRegister(ModuleName, FastCountFuncName, func() interfaces.Func { return &FastCount{} }) // must register the fact and name
}

// FastCount is a fact that counts up as fast as possible from zero forever.
type FastCount struct {
	init *interfaces.Init

	mutex *sync.Mutex
	count int
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *FastCount) String() string {
	return FastCountFuncName
}

// Validate makes sure we've built our struct properly.
func (obj *FastCount) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *FastCount) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() int"),
	}
}

// Init runs some startup code for this fact.
func (obj *FastCount) Init(init *interfaces.Init) error {
	obj.init = init
	obj.mutex = &sync.Mutex{}
	return nil
}

// Stream starts a mainloop and runs Event when it's time to Call() again.
func (obj *FastCount) Stream(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil

		default:
			// run free
		}

		obj.mutex.Lock()
		obj.count++
		obj.mutex.Unlock()

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *FastCount) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if obj.mutex == nil {
		return nil, funcs.ErrCantSpeculate
	}
	obj.mutex.Lock() // TODO: could be a read lock
	count := obj.count
	obj.mutex.Unlock()
	return &types.IntValue{
		V: count,
	}, nil
}

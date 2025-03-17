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

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// FastCountFuncName is the name this fact is registered as. It's still
	// a Func Name because this is the name space the fact is actually
	// using.
	FastCountFuncName = "fastcount"
)

func init() {
	facts.ModuleRegister(ModuleName, FastCountFuncName, func() facts.Fact { return &FastCountFact{} }) // must register the fact and name
}

// FastCountFact is a fact that counts up as fast as possible from zero forever.
type FastCountFact struct {
	init *facts.Init

	mutex *sync.Mutex
	count int
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *FastCountFact) String() string {
	return FastCountFuncName
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *FastCountFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *FastCountFact) Info() *facts.Info {
	return &facts.Info{
		Pure:   false,
		Memo:   false,
		Output: types.NewType("int"),
	}
}

// Init runs some startup code for this fact.
func (obj *FastCountFact) Init(init *facts.Init) error {
	obj.init = init
	obj.mutex = &sync.Mutex{}
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *FastCountFact) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // always signal when we're done

	// streams must generate an initial event on startup
	for {
		result, err := obj.Call(ctx)
		if err != nil {
			return err
		}

		obj.mutex.Lock()
		obj.count++
		obj.mutex.Unlock()

		select {
		case obj.init.Output <- result:

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *FastCountFact) Call(ctx context.Context) (types.Value, error) {
	if obj.mutex == nil {
		return nil, facts.ErrCantSpeculate
	}
	obj.mutex.Lock() // TODO: could be a read lock
	count := obj.count
	obj.mutex.Unlock()
	return &types.IntValue{
		V: int64(count),
	}, nil
}

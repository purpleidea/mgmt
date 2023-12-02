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

package coretest

import (
	"context"
	"sync"

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// OneInstanceAFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceAFuncName = "one_instance_a"

	// OneInstanceBFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceBFuncName = "one_instance_b"

	msg = "hello"
)

func init() {
	oneInstanceAMutex = &sync.Mutex{}
	oneInstanceBMutex = &sync.Mutex{}

	facts.ModuleRegister(ModuleName, OneInstanceAFuncName, func() facts.Fact { return &OneInstanceFact{} }) // must register the fact and name

	simple.ModuleRegister(ModuleName, OneInstanceBFuncName, &types.FuncValue{
		T: types.NewType("func() str"),
		V: func([]types.Value) (types.Value, error) {
			oneInstanceBMutex.Lock()
			if oneInstanceBFlag {
				panic("should not get called twice")
			}
			oneInstanceBFlag = true
			oneInstanceBMutex.Unlock()
			return &types.StrValue{V: msg}, nil
		},
	})
}

var (
	oneInstanceAFlag  bool
	oneInstanceBFlag  bool
	oneInstanceAMutex *sync.Mutex
	oneInstanceBMutex *sync.Mutex
)

// OneInstanceFact is a fact which flips a bool repeatedly. This is an example
// fact and is not meant for serious computing. This would be better served by a
// flip function which you could specify an interval for.
type OneInstanceFact struct {
	init *facts.Init
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *OneInstanceFact) String() string {
	return OneInstanceAFuncName
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal facts that users can use directly.
//func (obj *OneInstanceFact) Validate() error {
//	return nil
//}

// Info returns some static info about itself.
func (obj *OneInstanceFact) Info() *facts.Info {
	return &facts.Info{
		Output: types.NewType("str"),
	}
}

// Init runs some startup code for this fact.
func (obj *OneInstanceFact) Init(init *facts.Init) error {
	obj.init = init
	obj.init.Logf("Init of `%s` @ %p", OneInstanceAFuncName, obj)

	oneInstanceAMutex.Lock()
	if oneInstanceAFlag {
		panic("should not get called twice")
	}
	oneInstanceAFlag = true
	oneInstanceAMutex.Unlock()
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *OneInstanceFact) Stream(ctx context.Context) error {
	obj.init.Logf("Stream of `%s` @ %p", OneInstanceAFuncName, obj)
	defer close(obj.init.Output) // always signal when we're done
	select {
	case obj.init.Output <- &types.StrValue{
		V: msg,
	}:
	case <-ctx.Done():
		return nil
	}

	return nil
}

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
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/interfaces"
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

	// OneInstanceCFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceCFuncName = "one_instance_c"

	// OneInstanceDFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceDFuncName = "one_instance_d"

	// OneInstanceEFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceEFuncName = "one_instance_e"

	// OneInstanceFFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceFFuncName = "one_instance_f"

	// OneInstanceGFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceGFuncName = "one_instance_g"

	// OneInstanceHFuncName is the name this fact is registered as. It's
	// still a Func Name because this is the name space the fact is actually
	// using.
	OneInstanceHFuncName = "one_instance_h"

	msg = "hello"
)

func init() {
	oneInstanceAMutex = &sync.Mutex{}
	oneInstanceBMutex = &sync.Mutex{}
	oneInstanceCMutex = &sync.Mutex{}
	oneInstanceDMutex = &sync.Mutex{}
	oneInstanceEMutex = &sync.Mutex{}
	oneInstanceFMutex = &sync.Mutex{}
	oneInstanceGMutex = &sync.Mutex{}
	oneInstanceHMutex = &sync.Mutex{}

	funcs.ModuleRegister(ModuleName, OneInstanceAFuncName, func() interfaces.Func {
		return &OneInstance{
			Name:  OneInstanceAFuncName,
			Mutex: oneInstanceAMutex,
			Flag:  &oneInstanceAFlag,
		}
	}) // must register the fact and name
	funcs.ModuleRegister(ModuleName, OneInstanceCFuncName, func() interfaces.Func {
		return &OneInstance{
			Name:  OneInstanceCFuncName,
			Mutex: oneInstanceCMutex,
			Flag:  &oneInstanceCFlag,
		}
	})
	funcs.ModuleRegister(ModuleName, OneInstanceEFuncName, func() interfaces.Func {
		return &OneInstance{
			Name:  OneInstanceEFuncName,
			Mutex: oneInstanceEMutex,
			Flag:  &oneInstanceEFlag,
		}
	})
	funcs.ModuleRegister(ModuleName, OneInstanceGFuncName, func() interfaces.Func {
		return &OneInstance{
			Name:  OneInstanceGFuncName,
			Mutex: oneInstanceGMutex,
			Flag:  &oneInstanceGFlag,
		}
	})

	simple.ModuleRegister(ModuleName, OneInstanceBFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: false, // don't break tests
		},
		T: types.NewType("func() str"),
		F: func(context.Context, []types.Value) (types.Value, error) {
			oneInstanceBMutex.Lock()
			if oneInstanceBFlag {
				panic("should not get called twice")
			}
			oneInstanceBFlag = true
			oneInstanceBMutex.Unlock()
			return &types.StrValue{V: msg}, nil
		},
		D: &OneInstance{},
	})
	simple.ModuleRegister(ModuleName, OneInstanceDFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: false, // don't break tests
		},
		T: types.NewType("func() str"),
		F: func(context.Context, []types.Value) (types.Value, error) {
			oneInstanceDMutex.Lock()
			if oneInstanceDFlag {
				panic("should not get called twice")
			}
			oneInstanceDFlag = true
			oneInstanceDMutex.Unlock()
			return &types.StrValue{V: msg}, nil
		},
		D: &OneInstance{},
	})
	simple.ModuleRegister(ModuleName, OneInstanceFFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: false, // don't break tests
		},
		T: types.NewType("func() str"),
		F: func(context.Context, []types.Value) (types.Value, error) {
			oneInstanceFMutex.Lock()
			if oneInstanceFFlag {
				panic("should not get called twice")
			}
			oneInstanceFFlag = true
			oneInstanceFMutex.Unlock()
			return &types.StrValue{V: msg}, nil
		},
		D: &OneInstance{},
	})
	simple.ModuleRegister(ModuleName, OneInstanceHFuncName, &simple.Scaffold{
		I: &simple.Info{
			Pure: true,
			Memo: true,
			Fast: true,
			Spec: false, // don't break tests
		},
		T: types.NewType("func() str"),
		F: func(context.Context, []types.Value) (types.Value, error) {
			oneInstanceHMutex.Lock()
			if oneInstanceHFlag {
				panic("should not get called twice")
			}
			oneInstanceHFlag = true
			oneInstanceHMutex.Unlock()
			return &types.StrValue{V: msg}, nil
		},
		D: &OneInstance{},
	})
}

var (
	oneInstanceAFlag  bool
	oneInstanceBFlag  bool
	oneInstanceCFlag  bool
	oneInstanceDFlag  bool
	oneInstanceEFlag  bool
	oneInstanceFFlag  bool
	oneInstanceGFlag  bool
	oneInstanceHFlag  bool
	oneInstanceAMutex *sync.Mutex
	oneInstanceBMutex *sync.Mutex
	oneInstanceCMutex *sync.Mutex
	oneInstanceDMutex *sync.Mutex
	oneInstanceEMutex *sync.Mutex
	oneInstanceFMutex *sync.Mutex
	oneInstanceGMutex *sync.Mutex
	oneInstanceHMutex *sync.Mutex
)

// OneInstance is a fact which flips a bool repeatedly. This is an example fact
// and is not meant for serious computing. This would be better served by a flip
// function which you could specify an interval for.
type OneInstance struct {
	init *interfaces.Init

	Name  string
	Mutex *sync.Mutex
	Flag  *bool
}

// String returns a simple name for this fact. This is needed so this struct can
// satisfy the pgraph.Vertex interface.
func (obj *OneInstance) String() string {
	return obj.Name
}

// Validate makes sure we've built our struct properly.
func (obj *OneInstance) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *OneInstance) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // non-constant facts can't be pure!
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

// Init runs some startup code for this fact.
func (obj *OneInstance) Init(init *interfaces.Init) error {
	obj.init = init
	obj.init.Logf("Init of `%s` @ %p", obj.Name, obj)

	obj.Mutex.Lock()
	if *obj.Flag {
		panic("should not get called twice")
	}
	b := true
	obj.Flag = &b
	obj.Mutex.Unlock()
	return nil
}

// Call this fact and return the value if it is possible to do so at this time.
func (obj *OneInstance) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: msg,
	}, nil
}

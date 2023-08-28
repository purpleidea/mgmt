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

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// FastCountFuncName is the name this fact is registered as. It's still a
	// Func Name because this is the name space the fact is actually using.
	FastCountFuncName = "fastcount"
)

func init() {
	facts.ModuleRegister(ModuleName, FastCountFuncName, func() facts.Fact { return &FastCountFact{} }) // must register the fact and name
}

// FastCountFact is a fact that counts up as fast as possible from zero forever.
type FastCountFact struct {
	init *facts.Init
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
		Output: types.NewType("int"),
	}
}

// Init runs some startup code for this fact.
func (obj *FastCountFact) Init(init *facts.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this fact has over time.
func (obj *FastCountFact) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // always signal when we're done

	count := int64(0)

	// streams must generate an initial event on startup
	for {
		select {
		case obj.init.Output <- &types.IntValue{V: count}:
			count++

		case <-ctx.Done():
			return nil
		}
	}
}

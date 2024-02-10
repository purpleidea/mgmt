// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package facts

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// FactFunc is a wrapper for the fact interface. It implements the fact
// interface in terms of Func to reduce the two down to a single mechanism.
type FactFunc struct { // implements `interfaces.Func`
	Fact Fact
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *FactFunc) String() string {
	return obj.Fact.String()
}

// Validate makes sure we've built our struct properly.
func (obj *FactFunc) Validate() error {
	if obj.Fact == nil {
		return fmt.Errorf("must specify a Fact in struct")
	}
	//return obj.Fact.Validate() // currently unused
	return nil
}

// Info returns some static info about itself.
func (obj *FactFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Sig: &types.Type{
			Kind: types.KindFunc,
			// if Ord or Map are nil, this will panic things!
			Ord: []string{},
			Map: make(map[string]*types.Type),
			Out: obj.Fact.Info().Output,
		},
		Err: obj.Fact.Info().Err,
	}
}

// Init runs some startup code for this fact.
func (obj *FactFunc) Init(init *interfaces.Init) error {
	return obj.Fact.Init(
		&Init{
			Hostname: init.Hostname,
			Output:   init.Output,
			World:    init.World,
			Debug:    init.Debug,
			Logf:     init.Logf,
		},
	)
}

// Stream returns the changing values that this function has over time.
func (obj *FactFunc) Stream(ctx context.Context) error {
	return obj.Fact.Stream(ctx)
}

// Call means this function implements the CallableFunc interface and can be
// called statically if we want to do it speculatively or from a resource.
func (obj *FactFunc) Call() (types.Value, error) {

	callableFact, ok := obj.Fact.(CallableFact)
	if !ok {
		return nil, fmt.Errorf("fact is not a CallableFact")
	}

	return callableFact.Call()
}

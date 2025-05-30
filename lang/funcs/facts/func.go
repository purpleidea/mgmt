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

package facts

import (
	"context"
	"fmt"

	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// FactFunc is a wrapper for the fact interface. It implements the fact
// interface in terms of Func to reduce the two down to a single mechanism.
type FactFunc struct { // implements `interfaces.Func`
	*docsUtil.Metadata

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
		Pure: obj.Fact.Info().Pure,
		Memo: obj.Fact.Info().Memo,
		Fast: obj.Fact.Info().Fast,
		Spec: obj.Fact.Info().Spec,
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

// Call this fact and return the value if it is possible to do so at this time.
func (obj *FactFunc) Call(ctx context.Context, _ []types.Value) (types.Value, error) {
	//return obj.Fact.Call(ctx)

	callableFact, ok := obj.Fact.(CallableFact)
	if !ok {
		return nil, fmt.Errorf("fact is not a CallableFact")
	}

	return callableFact.Call(ctx)
}

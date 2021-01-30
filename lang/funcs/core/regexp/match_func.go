// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package coreregexp

import (
	"regexp"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	simple.ModuleRegister(ModuleName, "match", &types.FuncValue{
		T: types.NewType("func(pattern str, s str) bool"),
		V: Match,
	})
}

// Match matches whether a string matches the regexp pattern.
func Match(input []types.Value) (types.Value, error) {
	pattern := input[0].Str()
	s := input[1].Str()

	// TODO: We could make this more efficient with the regular function API
	// by only compiling the pattern when it changes.
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, errwrap.Wrapf(err, "pattern did not compile")
	}

	result := re.MatchString(s)
	return &types.BoolValue{
		V: result,
	}, nil
}

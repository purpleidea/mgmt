// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package simple // TODO: should this be in its own individual package?

import (
	"os"
	"strings"

	"github.com/purpleidea/mgmt/lang/types"
)

func init() {
	Register("getenv", &types.FuncValue{
		T: types.NewType("func(str) str"),
		V: GetEnv,
	})
	Register("defaultenv", &types.FuncValue{
		T: types.NewType("func(str, str) str"),
		V: DefaultEnv,
	})
	Register("hasenv", &types.FuncValue{
		T: types.NewType("func(str) bool"),
		V: HasEnv,
	})
	Register("env", &types.FuncValue{
		T: types.NewType("func() {str: str}"),
		V: Env,
	})
}

// GetEnv gets environment variable by name or returns empty string if non existing.
func GetEnv(input []types.Value) (types.Value, error) {
	return &types.StrValue{
		V: os.Getenv(input[0].Str()),
	}, nil
}

// DefaultEnv gets environment variable by name or returns default if non existing.
func DefaultEnv(input []types.Value) (types.Value, error) {
	value, exists := os.LookupEnv(input[0].Str())
	if !exists {
		value = input[1].Str()
	}
	return &types.StrValue{
		V: value,
	}, nil
}

// HasEnv returns true if environment variable exists.
func HasEnv(input []types.Value) (types.Value, error) {
	_, exists := os.LookupEnv(input[0].Str())
	return &types.BoolValue{
		V: exists,
	}, nil
}

// Env returns a map of all keys and their values.
func Env(input []types.Value) (types.Value, error) {
	environ := make(map[types.Value]types.Value)
	for _, keyval := range os.Environ() {
		if i := strings.IndexRune(keyval, '='); i != -1 {
			environ[&types.StrValue{V: keyval[:i]}] = &types.StrValue{V: keyval[i+1:]}
		}
	}
	return &types.MapValue{
		T: types.NewType("{str: str}"),
		V: environ,
	}, nil
}

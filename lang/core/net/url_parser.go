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

package corenet

import (
	"context"
	"fmt"
	"net/url"

	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const structComp = "struct{scheme str; host str; path str; query str}"

func init() {
	simple.ModuleRegister(ModuleName, "url_parser", &simple.Scaffold{
		T: types.NewType(fmt.Sprintf("func(str) %s", structComp)),
		F: URLParser,
	})
}

// URLParser takes an URL as a string, and finds the different components of
// said URL - scheme, host, path, and query. The function will error out if the
// given URL doesn't contain a scheme, as this is a limitation of the underlying
// net/url library.
func URLParser(ctx context.Context, input []types.Value) (types.Value, error) {
	i := input[0].Str()

	u, err := url.Parse(i)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error parsing the URL")
	}
	if u.Scheme == "" {
		return nil, errwrap.Wrapf(err, "empty schemes are invalid")
	}

	v := types.NewStruct(types.NewType(structComp))
	if err := v.Set("scheme", &types.StrValue{V: u.Scheme}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid scheme value")
	}
	if err := v.Set("host", &types.StrValue{V: u.Host}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid host value")
	}
	if err := v.Set("path", &types.StrValue{V: u.Path}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid path value")
	}
	if err := v.Set("query", &types.StrValue{V: u.RawQuery}); err != nil {
		return nil, errwrap.Wrapf(err, "invalid query value")
	}

	return v, nil
}

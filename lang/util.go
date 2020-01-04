// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

package lang

import (
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/funcs/simple"
	"github.com/purpleidea/mgmt/lang/funcs/simplepoly"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

// FuncPrefixToFunctionsScope is a helper function to return the functions
// portion of the scope from a function prefix lookup. Basically this wraps the
// implementation in the Func interface in the *ExprFunc struct.
func FuncPrefixToFunctionsScope(prefix string) map[string]interfaces.Expr {
	fns := funcs.LookupPrefix(prefix) // map[string]func() interfaces.Func
	exprs := make(map[string]interfaces.Expr)
	for name, f := range fns {

		x := f() // inspect
		// We can pass in Fns []*types.FuncValue for the simple and
		// simplepoly API's and avoid the double wrapping from the
		// simple/simplepoly API's to the main function api and back.
		if st, ok := x.(*simple.WrappedFunc); simple.DirectInterface && ok {
			fn := &ExprFunc{
				Title: name,

				Values: []*types.FuncValue{st.Fn}, // just one!
			}
			exprs[name] = fn
			continue
		} else if st, ok := x.(*simplepoly.WrappedFunc); simplepoly.DirectInterface && ok {
			fn := &ExprFunc{
				Title: name,

				Values: st.Fns,
			}
			exprs[name] = fn
			continue
		}

		fn := &ExprFunc{
			Title: name,
			// We need to pass in the constructor function, because
			// we'll need more than one copy of this function if it
			// is used in more than one place so we can build more.
			Function: f, // func() interfaces.Func
		}
		exprs[name] = fn
	}
	return exprs
}

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

package util

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// HasDuplicateTypes returns an error if the list of types is not unique.
func HasDuplicateTypes(typs []*types.Type) error {
	// FIXME: do this comparison in < O(n^2) ?
	for i, ti := range typs {
		for j, tj := range typs {
			if i == j {
				continue // don't compare to self
			}
			if ti.Cmp(tj) == nil {
				return fmt.Errorf("duplicate type of %+v found", ti)
			}
		}
	}
	return nil
}

// FnMatch is run to turn a polymorphic, undetermined list of functions, into a
// specific statically typed version. It is usually run after Unify completes.
// It returns the index of the matched function.
func FnMatch(typ *types.Type, fns []*types.FuncValue) (int, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ == nil {
		return 0, fmt.Errorf("type of function must be specified")
	}
	if typ.Kind != types.KindFunc {
		return 0, fmt.Errorf("type must be of kind Func")
	}
	if typ.Out == nil {
		return 0, fmt.Errorf("return type of function must be specified")
	}

	// find typ in fns
	for ix, f := range fns {
		if f.T.HasVariant() {
			continue // match these if no direct matches exist
		}
		// FIXME: can we replace this by the complex matcher down below?
		if f.T.Cmp(typ) == nil {
			return ix, nil // found match at this index
		}
	}

	// match concrete type against our list that might contain a variant
	var found bool
	var index int
	for ix, f := range fns {
		_, err := typ.ComplexCmp(f.T)
		if err != nil {
			continue
		}
		if found { // already found one...
			// TODO: we *could* check that the previous duplicate is
			// equivalent, but in this case, it is really a bug that
			// the function author had by allowing ambiguity in this
			return 0, fmt.Errorf("duplicate match found for build type: %+v", typ)
		}
		found = true
		index = ix // found match at this index
	}
	// ensure there's only one match...
	if found {
		return index, nil // w00t!
	}

	return 0, fmt.Errorf("unable to find a compatible function for type: %+v", typ)
}

// ValidateVarName returns an error if the string pattern does not match the
// format for a valid variable name. The leading dollar sign must not be passed
// in.
func ValidateVarName(name string) error {
	if name == "" {
		return fmt.Errorf("got empty var name")
	}

	// A variable always starts with an lowercase alphabetical char and
	// contains lowercase alphanumeric characters or numbers, underscores,
	// and non-consecutive dots. The last char must not be an underscore or
	// a dot.
	// TODO: put the variable matching pattern in a const somewhere?
	pattern := `^[a-z]([a-z0-9_]|(\.|_)[a-z0-9_])*$`

	matched, err := regexp.MatchString(pattern, name)
	if err != nil {
		return errwrap.Wrapf(err, "error matching regex")
	}
	if !matched {
		return fmt.Errorf("invalid var name: `%s`", name)
	}

	// Check that we don't get consecutive underscores or dots!
	// TODO: build this into the above regexp and into the parse.rl file!
	if strings.Contains(name, "..") {
		return fmt.Errorf("var name contains multiple periods: `%s`", name)
	}
	if strings.Contains(name, "__") {
		return fmt.Errorf("var name contains multiple underscores: `%s`", name)
	}

	return nil
}

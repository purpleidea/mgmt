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

package funcs

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
)

const (
	// ContainsFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	// XXX: change to _contains and add syntax in the lexer/parser
	ContainsFuncName = "contains"

	// arg names...
	containsArgNameNeedle   = "needle"
	containsArgNameHaystack = "haystack"
)

func init() {
	Register(ContainsFuncName, func() interfaces.Func { return &ContainsFunc{} }) // must register the func and name
}

var _ interfaces.BuildableFunc = &ContainsFunc{} // ensure it meets this expectation

// ContainsFunc returns true if a value is found in a list. Otherwise false.
type ContainsFunc struct {
	Type *types.Type // this is the type of value stored in our list

	init *interfaces.Init
	last types.Value // last value received to use for diff

	result types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ContainsFunc) String() string {
	return ContainsFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ContainsFunc) ArgGen(index int) (string, error) {
	seq := []string{containsArgNameNeedle, containsArgNameHaystack}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *ContainsFunc) sig() *types.Type {
	// func(needle ?1, haystack []?1) bool
	s := "?1"
	if obj.Type != nil { // don't panic if called speculatively
		s = obj.Type.String() // if solved
	}
	return types.NewType(fmt.Sprintf("func(%s %s, %s []%s) bool", containsArgNameNeedle, s, containsArgNameHaystack, s))
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ContainsFunc) Build(typ *types.Type) (*types.Type, error) {
	// We don't need to check that this matches, or that .Map has the right
	// length, because otherwise it would mean type unification is giving a
	// bad solution, which would be a major bug. Check to avoid any panics.
	// Other functions might need to check something if they only accept a
	// limited subset of the original type unification variables signature.
	//if err := unificationUtil.UnifyCmp(typ, obj.sig()); err != nil {
	//	return nil, err
	//}
	obj.Type = typ.Map[typ.Ord[0]] // type of value stored in our list
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ContainsFunc) Validate() error {
	if obj.Type == nil { // build must be run first
		return fmt.Errorf("type is still unspecified")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ContainsFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: true,
		Memo: false,
		Sig:  obj.sig(), // helper, func kind
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ContainsFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ContainsFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		case input, ok := <-obj.init.Input:
			if !ok {
				return nil // can't output any more
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			needle := input.Struct()[containsArgNameNeedle]
			haystack := (input.Struct()[containsArgNameHaystack]).(*types.ListValue)

			_, exists := haystack.Contains(needle)
			var result types.Value = &types.BoolValue{V: exists}

			// if previous input was `2 + 4`, but now it
			// changed to `1 + 5`, the result is still the
			// same, so we can skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
		case <-ctx.Done():
			return nil
		}
	}
}

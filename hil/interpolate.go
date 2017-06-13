// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package hil

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hil/ast"
)

// Variable defines an interpolated variable.
type Variable interface {
	Key() string
}

// ResourceVariable defines a variable type used to reference fields of a resource
// e.g.  ${file.file1.Content}
type ResourceVariable struct {
	Kind, Name, Field string
}

// Key returns a string representation of the variable key.
func (r *ResourceVariable) Key() string {
	return fmt.Sprintf("%s.%s.%s", r.Kind, r.Name, r.Field)
}

// NewInterpolatedVariable takes a variable key and return the interpolated variable
// of the required type.
func NewInterpolatedVariable(k string) (Variable, error) {
	// for now resource variables are the only thing.
	parts := strings.SplitN(k, ".", 3)

	return &ResourceVariable{
		Kind:  parts[0],
		Name:  parts[1],
		Field: parts[2],
	}, nil
}

// ParseVariables will traverse a HIL tree looking for variables and returns a
// list of them.
func ParseVariables(tree ast.Node) ([]Variable, error) {
	var result []Variable
	var finalErr error

	visitor := func(n ast.Node) ast.Node {
		if finalErr != nil {
			return n
		}

		switch nt := n.(type) {
		case *ast.VariableAccess:
			v, err := NewInterpolatedVariable(nt.Name)
			if err != nil {
				finalErr = err
				return n
			}
			result = append(result, v)
		default:
			return n
		}

		return n
	}

	tree.Accept(visitor)

	if finalErr != nil {
		return nil, finalErr
	}

	return result, nil
}

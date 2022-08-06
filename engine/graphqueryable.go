// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package engine

// GraphQueryableRes is the interface that must be implemented if you want your
// resource to be allowed to be queried from another resource in the graph. This
// is done as a form of explicit authorization tracking so that we can consider
// security aspects more easily. Ultimately, all resource code should be
// trusted, but it's still a good idea to know if a particular resource is even
// able to access information about another one, and if your resource doesn't
// add the trait supporting this, then it won't be allowed.
type GraphQueryableRes interface {
	Res // implement everything in Res but add the additional requirements

	// GraphQueryAllowed returns nil if you're allowed to query the graph.
	GraphQueryAllowed(...GraphQueryableOption) error
}

// GraphQueryableOption is an option that can be used to specify the
// authentication.
type GraphQueryableOption func(*GraphQueryableOptions)

// GraphQueryableOptions represents the different possible configurable options.
type GraphQueryableOptions struct {
	// Kind is the kind of the resource making the access.
	Kind string
	// Name is the name of the resource making the access.
	Name string
	// TODO: add more options if needed
}

// Apply is a helper function to apply a list of options to the struct. You
// should initialize it with defaults you want, and then apply any you've
// received like this.
func (obj *GraphQueryableOptions) Apply(opts ...GraphQueryableOption) {
	for _, optionFunc := range opts { // apply the options
		optionFunc(obj)
	}
}

// GraphQueryableOptionKind tells the GraphQueryAllowed function what the
// resource kind is.
func GraphQueryableOptionKind(kind string) GraphQueryableOption {
	return func(gqo *GraphQueryableOptions) {
		gqo.Kind = kind
	}
}

// GraphQueryableOptionName tells the GraphQueryAllowed function what the
// resource name is.
func GraphQueryableOptionName(name string) GraphQueryableOption {
	return func(gqo *GraphQueryableOptions) {
		gqo.Name = name
	}
}

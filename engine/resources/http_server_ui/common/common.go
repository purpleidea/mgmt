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

// Package common contains some code that is shared between the wasm and the
// http:server:ui packages.
package common

const (
	// HTTPServerUIInputType represents the field in the "Type" map that specifies
	// which input type we're using.
	HTTPServerUIInputType = "type"

	// HTTPServerUIInputTypeText is the representation of the html "text"
	// type.
	HTTPServerUIInputTypeText = "text"

	// HTTPServerUIInputTypeRange is the representation of the html "range"
	// type.
	HTTPServerUIInputTypeRange = "range"

	// HTTPServerUIInputTypeRangeMin is the html input "range" min field.
	HTTPServerUIInputTypeRangeMin = "min"

	// HTTPServerUIInputTypeRangeMax is the html input "range" max field.
	HTTPServerUIInputTypeRangeMax = "max"

	// HTTPServerUIInputTypeRangeStep is the html input "range" step field.
	HTTPServerUIInputTypeRangeStep = "step"
)

// Form represents the entire form containing all the desired elements.
type Form struct {
	// Elements is a list of form elements in this form.
	// TODO: Maybe this should be an interface?
	Elements []*FormElement `json:"elements"`
}

// FormElement represents each form element.
type FormElement struct {
	// Kind is the kind of form element that this is.
	Kind string `json:"kind"`

	// ID is the unique public id for this form element.
	ID string `json:"id"`

	// Type is a map that you can use to build the input field in the ui.
	Type map[string]string `json:"type"`

	// Sort is a string that you can use to determine the global sorted
	// display order of all the elements in a ui.
	Sort string `json:"sort"`
}

// FormElementGeneric is a value store.
type FormElementGeneric struct {
	// Value holds the string value we're interested in.
	Value string `json:"value"`
}

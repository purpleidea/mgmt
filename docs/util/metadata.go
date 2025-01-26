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

// Package util handles metadata for documentation generation.
package util

import (
	"fmt"
)

var (
	registeredResourceMetadata = make(map[string]*Metadata) // must initialize
	registeredFunctionMetadata = make(map[string]*Metadata) // must initialize
)

// RegisterResource records the metadata for a resource of this kind.
func RegisterResource(kind string, metadata *Metadata) error {
	if _, exists := registeredResourceMetadata[kind]; exists {
		return fmt.Errorf("metadata kind %s is already registered", kind)
	}

	registeredResourceMetadata[kind] = metadata

	return nil
}

// LookupResource looks up the metadata for a resource of this kind.
func LookupResource(kind string) (*Metadata, error) {
	metadata, exists := registeredResourceMetadata[kind]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return metadata, nil
}

// RegisterFunction records the metadata for a function of this name.
func RegisterFunction(name string, metadata *Metadata) error {
	if _, exists := registeredFunctionMetadata[name]; exists {
		return fmt.Errorf("metadata named %s is already registered", name)
	}

	registeredFunctionMetadata[name] = metadata

	return nil
}

// LookupFunction looks up the metadata for a function of this name.
func LookupFunction(name string) (*Metadata, error) {
	metadata, exists := registeredFunctionMetadata[name]
	if !exists {
		return nil, fmt.Errorf("not found")
	}
	return metadata, nil
}

// Metadata stores some additional information about the function or resource.
// This is used to automatically generate documentation.
type Metadata struct {
	// Filename is the filename (without any base dir path) that this is in.
	Filename string

	// Typename is the string name of the main resource struct or function.
	Typename string
}

// GetMetadata returns some metadata about the func. It can be called at any
// time. This must not be named the same as the struct it's on or using it as an
// anonymous embedded struct will stop us from being able to call this method.
func (obj *Metadata) GetMetadata() *Metadata {
	//if obj == nil { // TODO: Do I need this?
	//	return nil
	//}
	return &Metadata{
		Filename: obj.Filename,
		Typename: obj.Typename,
	}
}

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

package util

import (
	"fmt"
	"io/fs"

	"github.com/spf13/afero"
)

// AferoFs is a simple wrapper to a file system to be used for standalone
// deploys. This is basically a pass-through so that we fulfill the same
// interface that the deploy mechanism uses. If you give it Scheme and Path
// fields it will use those to build the URI. NOTE: This struct is here, since I
// don't know where else to put it for now.
type AferoFs struct {
	*afero.Afero

	Scheme string
	Path   string
}

// URI returns the unique URI of this filesystem. It returns the root path.
func (obj *AferoFs) URI() string {
	if obj.Scheme != "" {
		// if obj.Path is not empty and doesn't start with a slash, then
		// the first chunk will disappear when being parsed with stdlib
		return obj.Scheme + "://" + obj.Path
	}
	return fmt.Sprintf("%s://"+"/", obj.Name()) // old
}

// The constructors below are the only places where we're supposed to build one
// of our filesystems. They all return the same wrapper struct, which fulfills
// the engine.Fs interface, so that the rest of the code base never has to name
// the underlying implementation. This keeps the number of places which would
// need to change if we ever swap that out down to this one file. Set the Scheme
// and Path fields afterwards if this filesystem needs a specific URI.

// NewMemFs returns a new, empty filesystem which is stored in memory. It is
// used for tests, and to stage the files of a deploy before they get copied
// into the cluster.
func NewMemFs() *AferoFs {
	return &AferoFs{
		Afero: &afero.Afero{Fs: afero.NewMemMapFs()},
	}
}

// NewOsFs returns a filesystem which reads and writes to the local disk.
func NewOsFs() *AferoFs {
	return &AferoFs{
		Afero: &afero.Afero{Fs: afero.NewOsFs()},
	}
}

// NewReadOnlyOsFs returns a filesystem which reads from the local disk, and
// which errors on any attempt to write to it.
// TODO: Can we prevent access to any parent directory of a given base path?
func NewReadOnlyOsFs() *AferoFs {
	return &AferoFs{
		Afero: &afero.Afero{Fs: afero.NewReadOnlyFs(afero.NewOsFs())},
	}
}

// NewIOFs returns a read-only filesystem which reads from a standard library
// io/fs filesystem. This is how the mcl modules which are embedded into our
// binary get read. The paths in an io/fs are relative, so this re-roots them,
// which lets the caller use the absolute paths that we use everywhere else.
// XXX: All this horrible filesystem transformation mess happens because golang
// doesn't have a writeable io/fs.WriteableFS interface... We can eventually
// port this further away from Afero though...
func NewIOFs(fsys fs.ReadFileFS) *AferoFs {
	fromIOFS := afero.FromIOFS{FS: fsys}     // fulfills afero.Fs interface
	relPathFs := NewRelPathFs(fromIOFS, "/") // calls to `/foo` turn into `foo`
	return &AferoFs{
		Afero: &afero.Afero{Fs: relPathFs},
	}
}

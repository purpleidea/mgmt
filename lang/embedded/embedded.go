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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package embedded embeds mcl modules into the system import namespace.
// Typically these are made available via an `import "embedded/foo"` style stmt.
package embedded

import (
	"fmt"
	"io/fs"
	"runtime"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/util"

	"github.com/spf13/afero"
	"github.com/yalue/merged_fs"
)

const (
	// Scheme is the string used to represent the scheme used by the
	// embedded filesystem URI.
	Scheme = "embeddedfs"
)

var registeredEmbeds = make(map[string]fs.ReadFileFS) // must initialize

// ModuleRegister takes a filesystem and stores a reference to it in our
// embedded module system along with a name. Future lookups to that name will
// pull out that filesystem.
func ModuleRegister(module string, fs fs.ReadFileFS) {
	if _, exists := registeredEmbeds[module]; exists {
		panic(fmt.Sprintf("an embed in module %s is already registered", module))
	}

	// we currently set the fs URI when we return an fs with the Lookup func
	registeredEmbeds[module] = fs
}

// Lookup pulls out an embedded filesystem module which will contain a valid URI
// method. The returned fs is read-only.
// XXX: Update the interface to remove the afero part leaving this all read-only
func Lookup(module string) (engine.Fs, error) {
	fs, exists := registeredEmbeds[module]
	if !exists {
		return nil, fmt.Errorf("could not lookup embedded module: %s", module)
	}

	// XXX: All this horrible filesystem transformation mess happens because
	// golang doesn't have a writeable io/fs.WriteableFS interface... We can
	// eventually port this further away from Afero though...
	fromIOFS := afero.FromIOFS{FS: fs}     // fulfills afero.Fs interface
	rp := util.NewRelPathFs(fromIOFS, "/") // calls to `/foo` turn into `foo`
	afs := &afero.Afero{Fs: rp}            // wrap so that we're implementing ioutil
	engineFS := &util.AferoFs{             // fulfills engine.Fs interface
		Scheme: Scheme,       // pick the scheme!
		Path:   "/" + module, // need a leading slash
		Afero:  afs,
	}
	return engineFS, nil
}

// MergeFS merges multiple filesystems and returns an fs.FS. It is provided as a
// helper function to abstract away the underlying implementation in case we
// ever wish to replace it with something more performant or ergonomic.
// TODO: add a new interface that combines ReadFileFS and ReadDirFS and use that
// as the signature everywhere so we could catch those issues at the very start!
func MergeFS(filesystems ...fs.ReadFileFS) fs.ReadFileFS {
	l := []fs.FS{}
	for _, x := range filesystems {
		f, ok := x.(fs.FS)
		if !ok {
			// programming error
			panic("fs does not support basic FS")
		}
		l = append(l, f)
	}
	// runs NewMergedFS(a, b) in a balanced way recursively
	ret, ok := merged_fs.MergeMultiple(l...).(fs.ReadFileFS)
	if !ok {
		// programming error
		panic("fs does not support ReadFileFS")
	}
	return ret
}

// FullModuleName is a helper function that returns the embedded module name.
// This is the parent directory that an embedded code base should use as prefix.
func FullModuleName(moduleName string) string {
	pc, _, _, ok := runtime.Caller(1)
	if !ok {
		panic("caller info not found")
	}
	s := runtime.FuncForPC(pc).Name()
	chunks := strings.Split(s, "/")
	if len(chunks) < 2 {
		// programming error
		panic("split pattern not found")
	}
	name := chunks[len(chunks)-2]
	if name == "" {
		panic("name not found")
	}
	if moduleName == "" { // in case we only want to know the module name
		return name
	}
	return name + "/" + moduleName // do the concat for the user!
}

// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

package core

import (
	"embed"
	"io/fs"

	// import so the funcs register
	_ "github.com/purpleidea/mgmt/lang/funcs/core/convert"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/datetime"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/deploy"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/example"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/example/nested"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/fmt"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/iter"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/math"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/net"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/os"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/regexp"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/strings"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/sys"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/test"
	_ "github.com/purpleidea/mgmt/lang/funcs/core/world"
)

// TODO: Instead of doing this one-level embed, we could give each package an
// API that it calls to "register" the private embed.FS that it wants to share.

//go:embed */*.mcl
var mcl embed.FS

// AssetNames returns a flattened list of embedded .mcl file paths.
func AssetNames() ([]string, error) {
	fileSystem := mcl
	paths := []string{}
	err := fs.WalkDir(fileSystem, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() { // skip the dirs
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// Asset returns the contents of an embedded .mcl file.
func Asset(name string) ([]byte, error) {
	return mcl.ReadFile(name)
}

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

	"github.com/spf13/afero"
)

// Fs is a simple wrapper to a file system to be used for standalone deploys.
// This is basically a pass-through so that we fulfill the same interface that
// the deploy mechanism uses. To use this, wrap it with the implied field name,
// which will prevent `go vet` warnings, eg: `fs := &util.Fs{Afero: afs}`.
// NOTE: This struct is here, since I don't know where else to put it for now.
type Fs struct {
	*afero.Afero
}

// URI returns the unique URI of this filesystem. It returns the root path.
func (obj *Fs) URI() string { return fmt.Sprintf("%s://"+"/", obj.Name()) }

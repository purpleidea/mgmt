// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

import (
	"fmt"

	errwrap "github.com/pkg/errors"
)

// ResCopy copies a resource. This is the main entry point for copying a
// resource since it does all the common engine-level copying as well.
func ResCopy(r CopyableRes) (CopyableRes, error) {
	res := r.Copy()
	res.SetKind(r.Kind())
	res.SetName(r.Name())

	if x, ok := r.(MetaRes); ok {
		dst, ok := res.(MetaRes)
		if !ok {
			// programming error
			panic("meta interfaces are illogical")
		}
		dst.SetMetaParams(x.MetaParams().Copy()) // copy b/c we have it
	}

	if x, ok := r.(RefreshableRes); ok {
		dst, ok := res.(RefreshableRes)
		if !ok {
			// programming error
			panic("refresh interfaces are illogical")
		}
		dst.SetRefresh(x.Refresh()) // no need to copy atm
	}

	// copy meta params for resources with auto edges
	if x, ok := r.(EdgeableRes); ok {
		dst, ok := res.(EdgeableRes)
		if !ok {
			// programming error
			panic("autoedge interfaces are illogical")
		}
		dst.SetAutoEdgeMeta(x.AutoEdgeMeta()) // no need to copy atm
	}

	// copy meta params for resources with auto grouping
	if x, ok := r.(GroupableRes); ok {
		dst, ok := res.(GroupableRes)
		if !ok {
			// programming error
			panic("autogroup interfaces are illogical")
		}
		dst.SetAutoGroupMeta(x.AutoGroupMeta()) // no need to copy atm

		grouped := []GroupableRes{}
		for _, g := range x.GetGroup() {
			g0, ok := g.(CopyableRes)
			if !ok {
				return nil, fmt.Errorf("resource wasn't copyable")
			}
			g1, err := ResCopy(g0)
			if err != nil {
				return nil, err
			}
			g2, ok := g1.(GroupableRes)
			if !ok {
				return nil, fmt.Errorf("resource wasn't groupable")
			}
			grouped = append(grouped, g2)
		}
		dst.SetGroup(grouped)
	}

	if x, ok := r.(RecvableRes); ok {
		dst, ok := res.(RecvableRes)
		if !ok {
			// programming error
			panic("recv interfaces are illogical")
		}
		dst.SetRecv(x.Recv()) // no need to copy atm
	}

	if x, ok := r.(SendableRes); ok {
		dst, ok := res.(SendableRes)
		if !ok {
			// programming error
			panic("send interfaces are illogical")
		}
		if err := dst.Send(x.Sent()); err != nil { // no need to copy atm
			return nil, errwrap.Wrapf(err, "can't copy send")
		}
	}

	return res, nil
}

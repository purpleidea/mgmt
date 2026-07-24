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

package corenet

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// WiredInterfaceFuncName is the name this function is registered as.
	WiredInterfaceFuncName = "wired_interface"
)

func init() {
	funcs.ModuleRegister(ModuleName, WiredInterfaceFuncName, func() interfaces.Func {
		return &WiredInterfaceFunc{}
	})
}

// WiredInterfaceFunc returns the first wired interface and streams events when
// network links or addresses change.
type WiredInterfaceFunc struct {
	interfaces.Textarea

	init *interfaces.Init
}

// String returns a simple name for this function.
func (obj *WiredInterfaceFunc) String() string {
	return WiredInterfaceFuncName
}

// Validate makes sure the function was built correctly.
func (obj *WiredInterfaceFunc) Validate() error {
	return nil
}

// Info returns static information about this function.
func (obj *WiredInterfaceFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func() str"),
	}
}

// Init initializes this function.
func (obj *WiredInterfaceFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream emits an initial event and subsequent network change events.
func (obj *WiredInterfaceFunc) Stream(ctx context.Context) error {
	return networkEventStream(ctx, obj.init.Event)
}

// Call returns the current wired interface.
func (obj *WiredInterfaceFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	return WiredInterface(ctx, args)
}

// WiredInterface returns the first available non-loopback Ethernet interface.
// Interfaces identified as wireless by Linux sysfs are excluded. An interface
// which is already up is preferred.
func WiredInterface(ctx context.Context, input []types.Value) (types.Value, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not list interfaces")
	}

	fallback := ""
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}

		wirelessPath := filepath.Join("/sys/class/net", iface.Name, "wireless")
		if _, err := os.Stat(wirelessPath); err == nil {
			continue
		}

		if iface.Flags&net.FlagUp != 0 {
			return &types.StrValue{
				V: iface.Name,
			}, nil
		}
		if fallback == "" {
			fallback = iface.Name
		}
	}

	if fallback != "" {
		return &types.StrValue{
			V: fallback,
		}, nil
	}

	return nil, fmt.Errorf("no wired network interface found")
}

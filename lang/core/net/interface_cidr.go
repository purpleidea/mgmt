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

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// InterfaceCIDRFuncName is the name this function is registered as.
	InterfaceCIDRFuncName = "interface_cidr"

	interfaceAddrArgNameInterface = "interface"
)

func init() {
	funcs.ModuleRegister(ModuleName, InterfaceCIDRFuncName, func() interfaces.Func {
		return &InterfaceCIDRFunc{}
	})
}

// InterfaceCIDRFunc returns the IPv4 address and CIDR assigned to an interface
// and streams events when network links or addresses change.
type InterfaceCIDRFunc struct {
	interfaces.Textarea

	init *interfaces.Init
}

// String returns a simple name for this function.
func (obj *InterfaceCIDRFunc) String() string {
	return InterfaceCIDRFuncName
}

// ArgGen returns the Nth argument name for this function.
func (obj *InterfaceCIDRFunc) ArgGen(index int) (string, error) {
	seq := []string{interfaceAddrArgNameInterface}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure the function was built correctly.
func (obj *InterfaceCIDRFunc) Validate() error {
	return nil
}

// Info returns static information about this function.
func (obj *InterfaceCIDRFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType("func(interface str) str"),
	}
}

// Init initializes this function.
func (obj *InterfaceCIDRFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream emits an initial event and subsequent network change events.
func (obj *InterfaceCIDRFunc) Stream(ctx context.Context) error {
	return networkEventStream(ctx, obj.init.Event)
}

// Call returns the current address for the requested interface.
func (obj *InterfaceCIDRFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	return InterfaceCIDR(ctx, args)
}

// InterfaceCIDR returns the first global-unicast IPv4 address and CIDR prefix
// assigned to the named interface.
func InterfaceCIDR(ctx context.Context, input []types.Value) (types.Value, error) {
	name := input[0].Str()
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not find interface `%s`", name)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not list addresses on interface `%s`", name)
	}

	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil || ip.To4() == nil || !ip.IsGlobalUnicast() {
			continue
		}

		return &types.StrValue{
			V: addr.String(),
		}, nil
	}

	return nil, fmt.Errorf("interface `%s` has no global-unicast IPv4 address", name)
}

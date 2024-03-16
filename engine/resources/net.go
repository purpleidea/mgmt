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

//go:build !darwin

package resources

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"
	"github.com/purpleidea/mgmt/util/socketset"

	// XXX: Do NOT use subscribe methods from this lib, as they are racey
	// and do not clean up spawned goroutines. Should be replaced when a
	// suitable alternative is available.
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

func init() {
	engine.RegisterResource(KindNet, func() engine.Res { return &NetRes{} })

	// const.res.net.state.up = "up"
	// const.res.net.state.down = "down"
	vars.RegisterResourceParams(KindNet, map[string]map[string]func() interfaces.Var{
		ParamNetState: {
			NetStateUp: func() interfaces.Var {
				return &types.StrValue{
					V: NetStateUp,
				}
			},
			NetStateDown: func() interfaces.Var {
				return &types.StrValue{
					V: NetStateDown,
				}
			},
			// TODO: consider removing this field entirely
			"undefined": func() interfaces.Var {
				return &types.StrValue{
					V: NetStateUndefined, // empty string
				}
			},
		},
	})
}

const (
	// KindNet is the kind string used to identify this resource.
	KindNet = "net"

	// ParamNetState is the name of the state field parameter.
	ParamNetState = "state"

	// NetStateUp is the string that represents that the net state should be
	// up. This is the on interface state.
	NetStateUp = "up"

	// NetStateDown is the string that represents that the net state should
	// be down. This is the off interface state.
	NetStateDown = "down"

	// NetStateUndefined means the net state has not been specified.
	// TODO: consider moving to *string and express this state as a nil.
	NetStateUndefined = ""

	// IfacePrefix is the prefix used to identify unit files for managed
	// links.
	IfacePrefix = "mgmt-"

	// networkdUnitFileDir is the location of networkd unit files which
	// define the systemd network connections.
	networkdUnitFileDir = "/etc/systemd/network/"

	// networkdUnitFileExt is the file extension for networkd unit files.
	networkdUnitFileExt = ".network"

	// networkdUnitFileUmask sets the permissions on the systemd unit file.
	networkdUnitFileUmask = 0644

	// Netlink multicast groups to watch for events. For all groups see:
	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/rtnetlink.h
	rtmGrps           = rtmGrpLink | rtmGrpIPv4IfAddr | rtmGrpIPv6IfAddr | rtmGrpIPv4IfRoute
	rtmGrpLink        = 0x1   // interface create/delete/up/down
	rtmGrpIPv4IfAddr  = 0x10  // add/delete IPv4 addresses
	rtmGrpIPv6IfAddr  = 0x100 // add/delete IPv6 addresses
	rtmGrpIPv4IfRoute = 0x40  // add delete routes

	// IP routing protocols for used for netlink route messages. For all
	// protocols see:
	// https://github.com/torvalds/linux/blob/master/include/uapi/linux/rtnetlink.h
	rtProtoKernel = 2 // kernel
	rtProtoStatic = 4 // static

	socketFile = "pipe.sock" // path in vardir to store our socket file
)

// NetRes is a network interface resource based on netlink. It manages the state
// of a network link. Configuration is also stored in a networkd configuration
// file, so the network is available upon reboot. The name of the resource is
// the string representing the network interface name. This could be "eth0" for
// example. It supports flipping the state if you ask for it to be reversible.
type NetRes struct {
	traits.Base // add the base methods without re-implementation

	traits.Reversible

	init *engine.Init

	// State is the desired state of the interface. It can be "up", "down",
	// or the empty string to leave that unspecified.
	State string `lang:"state" yaml:"state"`

	// Addrs is the list of addresses to set on the interface. They must
	// each be in CIDR notation such as: 192.0.2.42/24 for example.
	Addrs []string `lang:"addrs" yaml:"addrs"`

	// Gateway represents the default route to set for the interface.
	Gateway string `lang:"gateway" yaml:"gateway"`

	// IPForward is a boolean that sets whether we should forward incoming
	// packets onward when this is set. It default to unspecified, which
	// downstream (in the systemd-networkd configuration) defaults to false.
	// XXX: this could also be "ipv4" or "ipv6", add those as a second option?
	IPForward *bool `lang:"ip_forward" yaml:"ip_forward"`

	iface        *iface // a struct containing the net.Interface and netlink.Link
	unitFilePath string // the interface unit file path

	socketFile string // path for storing the pipe socket file
}

// nlChanStruct defines the channel used to send netlink messages and errors to
// the event processing loop in Watch.
type nlChanStruct struct {
	msg []syscall.NetlinkMessage
	err error
}

// Default returns some sensible defaults for this resource.
func (obj *NetRes) Default() engine.Res {
	return &NetRes{}
}

// Validate if the params passed in are valid data.
func (obj *NetRes) Validate() error {
	// validate state
	if obj.State != NetStateUp && obj.State != NetStateDown && obj.State != "" {
		return fmt.Errorf("state must be up, down or empty")
	}

	// validate network address input
	if obj.Addrs != nil {
		for i, addr := range obj.Addrs {
			if _, _, err := net.ParseCIDR(addr); err != nil {
				if len(obj.Addrs) == 1 {
					return errwrap.Wrapf(err, "error parsing addr")
				}
				return errwrap.Wrapf(err, "error parsing addrs[%d]", i)
			}
		}
	}
	if obj.Gateway != "" {
		if g := net.ParseIP(obj.Gateway); g == nil {
			return fmt.Errorf("error parsing gateway: %s", obj.Gateway)
		}
	}

	// validate the interface name
	_, err := net.InterfaceByName(obj.Name())
	if err != nil {
		return errwrap.Wrapf(err, "error finding interface: %s", obj.Name())
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *NetRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	var err error

	// tmp directory for pipe socket
	dir, err := obj.init.VarDir("")
	if err != nil {
		return errwrap.Wrapf(err, "could not get VarDir in Init()")
	}
	obj.socketFile = path.Join(dir, socketFile) // return a unique file

	// store the network interface in the struct
	obj.iface = &iface{}
	if obj.iface.iface, err = net.InterfaceByName(obj.Name()); err != nil {
		return errwrap.Wrapf(err, "error finding interface: %s", obj.Name())
	}
	// store the netlink link to use as interface input in netlink functions
	if obj.iface.link, err = netlink.LinkByName(obj.Name()); err != nil {
		return errwrap.Wrapf(err, "error finding link: %s", obj.Name())
	}

	// build the path to the networkd configuration file
	obj.unitFilePath = networkdUnitFileDir + IfacePrefix + obj.Name() + networkdUnitFileExt

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *NetRes) Cleanup() error {
	if obj.socketFile == "/" {
		return fmt.Errorf("socket file should not be the root path")
	}
	if obj.socketFile != "" { // safety
		if err := os.Remove(obj.socketFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Watch listens for events from the specified interface via a netlink socket.
// TODO: currently gets events from ALL interfaces, would be nice to reject
// events from other interfaces.
func (obj *NetRes) Watch(ctx context.Context) error {
	// create a netlink socket for receiving network interface events
	conn, err := socketset.NewSocketSet(rtmGrps, obj.socketFile, unix.NETLINK_ROUTE)
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}

	// waitgroup for netlink receive goroutine
	wg := &sync.WaitGroup{}
	defer conn.Close()
	// We must wait for the Shutdown() AND the select inside of SocketSet to
	// complete before we Close, since the unblocking in SocketSet is not a
	// synchronous operation.
	defer wg.Wait()
	defer conn.Shutdown() // close the netlink socket and unblock conn.receive()

	// watch the systemd-networkd configuration file
	recWatcher, err := recwatch.NewRecWatcher(obj.unitFilePath, false)
	if err != nil {
		return err
	}
	// close the recwatcher when we're done
	defer recWatcher.Close()

	// channel for netlink messages
	nlChan := make(chan *nlChanStruct) // closed from goroutine

	// channel to unblock selects in goroutine
	closeChan := make(chan struct{})
	defer close(closeChan)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(nlChan)
		for {
			// receive messages from the socket set
			msgs, err := conn.ReceiveNetlinkMessages()
			if err != nil {
				select {
				case nlChan <- &nlChanStruct{
					err: errwrap.Wrapf(err, "error receiving messages"),
				}:
				case <-closeChan:
					return
				}
			}
			select {
			case nlChan <- &nlChanStruct{
				msg: msgs,
			}:
			case <-closeChan:
				return
			}
		}
	}()

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	var done bool
	for {
		select {
		case s, ok := <-nlChan:
			if !ok {
				if done {
					return nil
				}
				done = true
				continue
			}
			if err := s.err; err != nil {
				return errwrap.Wrapf(s.err, "unknown netlink error")
			}
			if obj.init.Debug {
				obj.init.Logf("Event: %+v", s.msg)
			}

			send = true

		case event, ok := <-recWatcher.Events():
			if !ok {
				if done {
					return nil
				}
				done = true
				continue
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "unknown recwatcher error")
			}
			if obj.init.Debug {
				obj.init.Logf("Event(%s): %v", event.Body.Name, event.Body.Op)
			}

			send = true

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// ifaceCheckApply checks the state of the network device and brings it up or
// down as necessary.
func (obj *NetRes) ifaceCheckApply(ctx context.Context, apply bool) (bool, error) {
	// check the interface state
	state, err := obj.iface.state()
	if err != nil {
		return false, errwrap.Wrapf(err, "error checking %s state", obj.Name())
	}
	// if the state is correct or unspecified, we're done
	if obj.State == state || obj.State == "" {
		return true, nil
	}

	// end of state checking
	if !apply {
		return false, nil
	}
	obj.init.Logf("ifaceCheckApply(%t)", apply)

	// ip link set up/down
	if err := obj.iface.linkUpDown(obj.State); err != nil {
		return false, errwrap.Wrapf(err, "error setting %s up or down", obj.Name())
	}

	return false, nil
}

// addrCheckApply checks if the interface has the correct addresses and then
// adds/deletes addresses as necessary.
func (obj *NetRes) addrCheckApply(ctx context.Context, apply bool) (bool, error) {
	// get the link's addresses
	ifaceAddrs, err := obj.iface.getAddrs()
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting addresses from %s", obj.Name())
	}
	// if state is not defined
	if obj.Addrs == nil {
		// send addrs
		obj.Addrs = ifaceAddrs
		return true, nil
	}
	// check if all addrs have a kernel route needed for first hop
	kernelOK, err := obj.iface.kernelCheck(obj.Addrs)
	if err != nil {
		return false, errwrap.Wrapf(err, "error checking kernel routes")
	}

	// if the kernel routes are intact and the addrs match, we're done
	err = util.SortedStrSliceCompare(obj.Addrs, ifaceAddrs)
	if err == nil && kernelOK {
		return true, nil
	}

	// end of state checking
	if !apply {
		return false, nil
	}
	obj.init.Logf("addrCheckApply(%t)", apply)

	// check each address and delete the ones that aren't in the definition
	if err := obj.iface.addrApplyDelete(obj.Addrs); err != nil {
		return false, errwrap.Wrapf(err, "error checking or deleting addresses")
	}
	// check each address and add the ones that are defined but do not exist
	if err := obj.iface.addrApplyAdd(obj.Addrs); err != nil {
		return false, errwrap.Wrapf(err, "error checking or adding addresses")
	}
	// make sure all the addrs have the appropriate kernel routes
	if err := obj.iface.kernelApply(obj.Addrs); err != nil {
		return false, errwrap.Wrapf(err, "error adding kernel routes")
	}

	return false, nil
}

// gatewayCheckApply checks if the interface has the correct default gateway and
// adds/deletes routes as necessary.
func (obj *NetRes) gatewayCheckApply(ctx context.Context, apply bool) (bool, error) {
	// get all routes from the interface
	routes, err := netlink.RouteList(obj.iface.link, netlink.FAMILY_V4)
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting default routes")
	}
	// add default routes to a slice
	defRoutes := []netlink.Route{}
	for _, route := range routes {
		if route.Dst == nil { // route is default
			defRoutes = append(defRoutes, route)
		}
	}
	// if the gateway is already set, we're done
	if len(defRoutes) == 1 && defRoutes[0].Gw.String() == obj.Gateway {
		return true, nil
	}
	// if no gateway was defined
	if obj.Gateway == "" {
		// send the gateway if there is one
		if len(defRoutes) == 1 {
			obj.Gateway = defRoutes[0].Gw.String()
		}
		return true, nil
	}

	// end of state checking
	if !apply {
		return false, nil
	}
	obj.init.Logf("gatewayCheckApply(%t)", apply)

	// delete all but one default route
	for i := 1; i < len(defRoutes); i++ {
		if err := netlink.RouteDel(&defRoutes[i]); err != nil {
			return false, errwrap.Wrapf(err, "error deleting route: %+v", defRoutes[i])
		}
	}

	// add or change the default route
	if err := netlink.RouteReplace(&netlink.Route{
		LinkIndex: obj.iface.iface.Index,
		Gw:        net.ParseIP(obj.Gateway),
		Protocol:  rtProtoStatic,
	}); err != nil {
		return false, errwrap.Wrapf(err, "error replacing default route")
	}

	return false, nil
}

// fileCheckApply checks and maintains the systemd-networkd unit file contents.
// TODO: replace this with a file resource if we can do so cleanly.
func (obj *NetRes) fileCheckApply(ctx context.Context, apply bool) (bool, error) {
	// if the state is unspecified, we're done
	if obj.State == "" {
		return true, nil
	}

	// check if the unit file exists
	_, err := os.Stat(obj.unitFilePath)
	if err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "error checking file")
	}

	exists := err == nil
	if obj.State == NetStateDown && !exists {
		return true, nil
	}

	fileContents := []byte{}
	if exists {
		// check the file contents
		byt, err := os.ReadFile(obj.unitFilePath)
		if err != nil {
			return false, errwrap.Wrapf(err, "error reading file")
		}
		fileContents = byt
	}
	// build the unit file contents from the definition
	contents := obj.unitFileContents()

	if obj.State == NetStateUp && exists && bytes.Equal(fileContents, contents) {
		return true, nil
	}

	if !apply {
		return false, nil
	}
	obj.init.Logf("fileCheckApply(%t)", apply)

	if obj.State == NetStateDown && exists {
		if err := os.Remove(obj.unitFilePath); err != nil {
			return false, errwrap.Wrapf(err, "error removing configuration file")
		}
		return false, nil
	}

	// all other situations, we write the file
	if err := os.WriteFile(obj.unitFilePath, contents, networkdUnitFileUmask); err != nil {
		return false, errwrap.Wrapf(err, "error writing configuration file")
	}
	return false, nil
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *NetRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	checkOK := true

	// check the networkd unit file
	if c, err := obj.fileCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// check the network device
	if c, err := obj.ifaceCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// if the interface is supposed to be down, we're done
	if obj.State == NetStateDown {
		return checkOK, nil
	}

	// check the addresses
	if c, err := obj.addrCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// check the gateway
	if c, err := obj.gatewayCheckApply(ctx, apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *NetRes) Cmp(r engine.Res) error {
	// we can only compare NetRes to others of the same resource kind
	res, ok := r.(*NetRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if (obj.Addrs == nil) != (res.Addrs == nil) {
		return fmt.Errorf("the Addrs differ")
	}
	if err := util.SortedStrSliceCompare(obj.Addrs, res.Addrs); err != nil {
		return fmt.Errorf("the Addrs differ")
	}
	if obj.Gateway != res.Gateway {
		return fmt.Errorf("the Gateway differs")
	}

	return nil
}

// NetUID is a unique resource identifier.
type NetUID struct {
	// NOTE: There is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	engine.BaseUID

	name string // the network interface name
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *NetUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*NetUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one although some resources can return multiple.
func (obj *NetRes) UIDs() []engine.ResUID {
	x := &NetUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *NetRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes NetRes // indirection to avoid infinite recursion

	def := obj.Default()     // get the default
	res, ok := def.(*NetRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to NetRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = NetRes(raw) // restore from indirection with type conversion!
	return nil
}

// Copy copies the resource. Don't call it directly, use engine.ResCopy instead.
// TODO: should this copy internal state?
func (obj *NetRes) Copy() engine.CopyableRes {
	addrs := []string{}
	for _, addr := range obj.Addrs {
		addrs = append(addrs, addr)
	}
	var ipforward *bool
	if obj.IPForward != nil { // copy the content, not the pointer...
		b := *obj.IPForward
		ipforward = &b
	}
	return &NetRes{
		State:     obj.State,
		Addrs:     addrs,
		Gateway:   obj.Gateway,
		IPForward: ipforward,
	}
}

// Reversed returns the "reverse" or "reciprocal" resource. This is used to
// "clean" up after a previously defined resource has been removed.
func (obj *NetRes) Reversed() (engine.ReversibleRes, error) {
	cp, err := engine.ResCopy(obj)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not copy")
	}
	rev, ok := cp.(engine.ReversibleRes)
	if !ok {
		return nil, fmt.Errorf("not reversible")
	}
	rev.ReversibleMeta().Disabled = true // the reverse shouldn't run again

	res, ok := cp.(*NetRes)
	if !ok {
		return nil, fmt.Errorf("copied res was not our kind")
	}

	// Only one field to reverse for now. It also removes the config file.
	if obj.State == NetStateUp {
		res.State = NetStateDown
	}
	if obj.State == NetStateDown {
		res.State = NetStateUp
	}

	return res, nil
}

// unitFileContents builds the unit file contents from the definition.
func (obj *NetRes) unitFileContents() []byte {
	// build the unit file contents
	u := []string{"[Match]"}
	u = append(u, fmt.Sprintf("Name=%s", obj.Name()))
	u = append(u, "[Network]")
	for _, addr := range obj.Addrs {
		u = append(u, fmt.Sprintf("Address=%s", addr))
	}
	if obj.Gateway != "" {
		u = append(u, fmt.Sprintf("Gateway=%s", obj.Gateway))
	}
	if obj.IPForward != nil {
		b := "false"
		if *obj.IPForward {
			b = "true"
		}
		u = append(u, fmt.Sprintf("IPForward=%s", b))
	}
	c := strings.Join(u, "\n")
	return []byte(c)
}

// iface wraps net.Interface to add additional methods.
type iface struct {
	iface *net.Interface
	link  netlink.Link
}

// state reports the state of the interface as up or down.
func (obj *iface) state() (string, error) {
	var err error
	if obj.iface, err = net.InterfaceByName(obj.iface.Name); err != nil {
		return "", errwrap.Wrapf(err, "error updating interface")
	}
	// if the interface's "up" flag is 0, it's down
	if obj.iface.Flags&net.FlagUp == 0 {
		return NetStateDown, nil
	}
	// otherwise it's up
	return NetStateUp, nil
}

// linkUpDown brings the interface up or down, depending on input value.
func (obj *iface) linkUpDown(state string) error {
	if state != NetStateUp && state != NetStateDown {
		return fmt.Errorf("state must be up or down")
	}
	if state == NetStateUp {
		return netlink.LinkSetUp(obj.link)
	}
	return netlink.LinkSetDown(obj.link)
}

// getAddrs returns a list of strings containing all of the interface's IP
// addresses in CIDR format.
func (obj *iface) getAddrs() ([]string, error) {
	var ifaceAddrs []string
	a, err := obj.iface.Addrs()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error getting addrs from interface: %s", obj.iface.Name)
	}
	// we're only interested in the strings (not the network)
	for _, addr := range a {
		ifaceAddrs = append(ifaceAddrs, addr.String())
	}
	return ifaceAddrs, nil
}

// kernelCheck checks if all addresses in the list have a corresponding kernel
// route, without which the network would be unreachable.
func (obj *iface) kernelCheck(addrs []string) (bool, error) {
	var routeOK bool

	// get a list of all the routes associated with the interface
	routes, err := netlink.RouteList(obj.link, netlink.FAMILY_V4)
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting routes")
	}
	// check each route against each addr
	for i, addr := range addrs {
		routeOK = false
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			if len(addrs) == 1 {
				return false, errwrap.Wrapf(err, "error parsing addr")
			}
			return false, errwrap.Wrapf(err, "error parsing addrs[%d]", i)
		}
		for _, r := range routes {
			// if src, dst and protocol are correct, the kernel route exists
			if r.Src.Equal(ip) && r.Dst.String() == ipNet.String() && r.Protocol == rtProtoKernel {
				routeOK = true
				break
			}
		}
		// if any addr is missing a kernel route return early
		if !routeOK {
			break
		}
	}
	return routeOK, nil
}

// kernelApply adds or replaces each address' kernel route as necessary.
func (obj *iface) kernelApply(addrs []string) error {
	// for each addr, add or replace the corresponding kernel route
	for i, addr := range addrs {
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			if len(addrs) == 1 {
				return errwrap.Wrapf(err, "error parsing addr")
			}
			return errwrap.Wrapf(err, "error parsing addrs[%d]", i)
		}
		// kernel route needed for the network to be reachable from a given ip
		if err := netlink.RouteReplace(&netlink.Route{
			LinkIndex: obj.iface.Index,
			Dst:       ipNet,
			Src:       ip,
			Protocol:  rtProtoKernel,
			Scope:     netlink.SCOPE_LINK,
		}); err != nil {
			return errwrap.Wrapf(err, "error replacing first hop route")
		}
	}
	return nil
}

// addrApplyDelete, checks the interface's addresses and deletes any that are
// not in the list/definition.
func (obj *iface) addrApplyDelete(objAddrs []string) error {
	ifaceAddrs, err := obj.getAddrs()
	if err != nil {
		return errwrap.Wrapf(err, "error getting addrs from interface: %s", obj.iface.Name)
	}
	for _, ifaceAddr := range ifaceAddrs {
		addrOK := false
		for _, objAddr := range objAddrs {
			if ifaceAddr == objAddr {
				addrOK = true
			}
		}
		if addrOK {
			continue
		}
		addr, err := netlink.ParseAddr(ifaceAddr)
		if err != nil {
			return errwrap.Wrapf(err, "error parsing netlink address: %s", ifaceAddr)
		}
		if err := netlink.AddrDel(obj.link, addr); err != nil {
			return errwrap.Wrapf(err, "error deleting addr: %s from %s", ifaceAddr, obj.iface.Name)
		}
	}
	return nil
}

// addrApplyAdd checks if the interface has each address in the supplied list,
// and if it doesn't, it adds them.
func (obj *iface) addrApplyAdd(objAddrs []string) error {
	ifaceAddrs, err := obj.getAddrs()
	if err != nil {
		return errwrap.Wrapf(err, "error getting addrs from interface: %s", obj.iface.Name)
	}
	for _, objAddr := range objAddrs {
		addrOK := false
		for _, ifaceAddr := range ifaceAddrs {
			if ifaceAddr == objAddr {
				addrOK = true
			}
		}
		if addrOK {
			continue
		}
		addr, err := netlink.ParseAddr(objAddr)
		if err != nil {
			return errwrap.Wrapf(err, "error parsing cidr address: %s", objAddr)
		}
		if err := netlink.AddrAdd(obj.link, addr); err != nil {
			return errwrap.Wrapf(err, "error adding addr: %s to %s", objAddr, obj.iface.Name)
		}
	}
	return nil
}

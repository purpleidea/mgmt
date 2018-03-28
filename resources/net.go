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

package resources

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/purpleidea/mgmt/recwatch"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	// XXX: Do NOT use subscribe methods from this lib, as they are racey and
	// do not clean up spawned goroutines. Should be replaced when a suitable
	// alternative is available.
	"github.com/vishvananda/netlink"
)

func init() {
	RegisterResource("net", func() Res { return &NetRes{} })
}

const (
	// IfacePrefix is the prefix used to identify unit files for managed links.
	IfacePrefix = "mgmt-"
	// networkdUnitFileDir is the location of networkd unit files which define
	// the systemd network connections.
	networkdUnitFileDir = "/etc/systemd/network/"
	// networkdUnitFileExt is the file extension for networkd unit files.
	networkdUnitFileExt = ".network"
	// networkdUnitFileUmask sets the permissions on the systemd unit file.
	networkdUnitFileUmask = 0644

	// ifaceUp is the up (on) interface state.
	ifaceUp = "up"
	// ifaceDown is the down (off) interface state.
	ifaceDown = "down"

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
)

// NetRes is a network interface resource based on netlink. It manages the
// state of a network link. Configuration is also stored in a networkd
// configuration file, so the network is available upon reboot.
type NetRes struct {
	BaseRes `yaml:",inline"`
	State   string   `yaml:"state"`   // up, down, or empty
	Addrs   []string `yaml:"addrs"`   // list of addresses in cidr format
	Gateway string   `yaml:"gateway"` // gateway address

	iface        *iface // a struct containing the net.Interface and netlink.Link
	unitFilePath string // the interface unit file path
	// XXX: replace TempDir with VarDir
	tempDir string // temporary directory for storing the pipe socket file
}

// nlChanStruct defines the channel used to send netlink messages and errors
// to the event processing loop in Watch.
type nlChanStruct struct {
	msg []syscall.NetlinkMessage
	err error
}

// Default returns some sensible defaults for this resource.
func (obj *NetRes) Default() Res {
	return &NetRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
	}
}

// Validate if the params passed in are valid data.
func (obj *NetRes) Validate() error {
	// validate state
	if obj.State != ifaceUp && obj.State != ifaceDown && obj.State != "" {
		return fmt.Errorf("state must be up, down or empty")
	}

	// validate network address input
	if (obj.Addrs == nil) != (obj.Gateway == "") {
		return fmt.Errorf("addrs and gateway must both be set or both be empty")
	}
	if obj.Addrs != nil {
		for _, addr := range obj.Addrs {
			if _, _, err := net.ParseCIDR(addr); err != nil {
				return errwrap.Wrapf(err, "error parsing address: %s", addr)
			}
		}
	}
	if obj.Gateway != "" {
		if g := net.ParseIP(obj.Gateway); g == nil {
			return fmt.Errorf("error parsing gateway: %s", obj.Gateway)
		}
	}

	// validate the interface name
	_, err := net.InterfaceByName(obj.GetName())
	if err != nil {
		return errwrap.Wrapf(err, "error finding interface: %s", obj.GetName())
	}

	return obj.BaseRes.Validate()
}

// Init runs some startup code for this resource.
func (obj *NetRes) Init() error {
	var err error
	// tmp directory for pipe socket
	// XXX: Replace with obj.VarDir
	if obj.tempDir, err = ioutil.TempDir("", "pipe"); err != nil {
		return errwrap.Wrapf(err, "could not get TempDir")
	}

	// store the network interface in the struct
	obj.iface = &iface{}
	if obj.iface.iface, err = net.InterfaceByName(obj.GetName()); err != nil {
		return errwrap.Wrapf(err, "error finding interface: %s", obj.GetName())
	}
	// store the netlink link to use as interface input in netlink functions
	if obj.iface.link, err = netlink.LinkByName(obj.GetName()); err != nil {
		return errwrap.Wrapf(err, "error finding link: %s", obj.GetName())
	}

	// build the path to the networkd configuration file
	obj.unitFilePath = networkdUnitFileDir + IfacePrefix + obj.GetName() + networkdUnitFileExt

	return obj.BaseRes.Init()
}

// Watch listens for events from the specified interface via a netlink socket.
// TODO: currently gets events from ALL interfaces, would be nice to reject
// events from other interfaces.
func (obj *NetRes) Watch() error {
	// waitgroup for netlink receive goroutine
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	// create a netlink socket for receiving network interface events
	conn, err := newSocketSet(rtmGrps, path.Join(obj.tempDir, "pipe.sock"))
	if err != nil {
		return errwrap.Wrapf(err, "error creating socket set")
	}
	defer conn.shutdown() // close the netlink socket and unblock conn.receive()

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
		defer conn.close() // close the pipe when we're done with it
		defer close(nlChan)
		for {
			// receive messages from the socket set
			msgs, err := conn.receive()
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

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var exit *error
	var send bool
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
			if obj.debug {
				log.Printf("%s: Event: %+v", obj, s.msg)
			}

			send = true
			obj.StateOK(false)

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
			if obj.debug {
				log.Printf("%s: Event(%s): %v", obj, event.Body.Name, event.Body.Op)
			}

			send = true
			obj.StateOK(false) // dirty

		case event := <-obj.Events():
			if exit, send = obj.ReadEvent(event); exit != nil {
				return *exit // exit
			}
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.Event()
		}
	}
}

// ifaceCheckApply checks the state of the network device and brings it up or
// down as necessary.
func (obj *NetRes) ifaceCheckApply(apply bool) (bool, error) {
	// check the interface state
	state, err := obj.iface.state()
	if err != nil {
		return false, errwrap.Wrapf(err, "error checking %s state", obj.GetName())
	}
	// if the state is correct or unspecified, we're done
	if obj.State == state || obj.State == "" {
		return true, nil
	}

	// end of state checking
	if !apply {
		return false, nil
	}
	log.Printf("%s: ifaceCheckApply(%t)", obj, apply)

	// ip link set up/down
	if err := obj.iface.linkUpDown(obj.State); err != nil {
		return false, errwrap.Wrapf(err, "error setting %s up or down", obj.GetName())
	}

	return false, nil
}

// addrCheckApply checks if the interface has the correct addresses and then
// adds/deletes addresses as necessary.
func (obj *NetRes) addrCheckApply(apply bool) (bool, error) {
	// get the link's addresses
	ifaceAddrs, err := obj.iface.getAddrs()
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting addresses from %s", obj.GetName())
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
	err = StrSortedSliceCompare(obj.Addrs, ifaceAddrs)
	if err == nil && kernelOK {
		return true, nil
	}

	// end of state checking
	if !apply {
		return false, nil
	}
	log.Printf("%s: addrCheckApply(%t)", obj, apply)

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

// gatewayCheckApply checks if the interface has the correct default gateway
// and adds/deletes routes as necessary.
func (obj *NetRes) gatewayCheckApply(apply bool) (bool, error) {
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
	log.Printf("%s: gatewayCheckApply(%t)", obj, apply)

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
func (obj *NetRes) fileCheckApply(apply bool) (bool, error) {
	// check if the unit file exists
	_, err := os.Stat(obj.unitFilePath)
	if err != nil && !os.IsNotExist(err) {
		return false, errwrap.Wrapf(err, "error checking file")
	}
	// build the unit file contents from the definition
	contents := obj.unitFileContents()
	// check the file contents
	if err == nil {
		unitFile, err := ioutil.ReadFile(obj.unitFilePath)
		if err != nil {
			return false, errwrap.Wrapf(err, "error reading file")
		}
		// return if the file is good
		if bytes.Equal(unitFile, contents) {
			return true, nil
		}
	}

	if !apply {
		return false, nil
	}
	log.Printf("%s: fileCheckApply(%t)", obj, apply)

	// write the file
	if err := ioutil.WriteFile(obj.unitFilePath, contents, networkdUnitFileUmask); err != nil {
		return false, errwrap.Wrapf(err, "error writing configuration file")
	}
	return false, nil
}

// CheckApply is run to check the state and, if apply is true, to apply the
// necessary changes to reach the desired state. This is run before Watch and
// again if Watch finds a change occurring to the state.
func (obj *NetRes) CheckApply(apply bool) (checkOK bool, err error) {
	checkOK = true

	// check the network device
	if c, err := obj.ifaceCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// if the interface is supposed to be down, we're done
	if obj.State == ifaceDown {
		return checkOK, nil
	}

	// check the addresses
	if c, err := obj.addrCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// check the gateway
	if c, err := obj.gatewayCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// if the state is unspecified, we're done
	if obj.State == "" {
		return checkOK, nil
	}

	// check the networkd unit file
	if c, err := obj.fileCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil
}

// Close cleans up when we're done.
func (obj *NetRes) Close() error {
	var errList error
	// XXX: replace TempDir with VarDir
	if err := os.RemoveAll(obj.tempDir); err != nil {
		errList = multierr.Append(errList, err)
	}
	if err := obj.BaseRes.Close(); err != nil {
		errList = multierr.Append(errList, err)
	}
	return errList
}

// NetUID is a unique resource identifier.
type NetUID struct {
	// NOTE: There is also a name variable in the BaseUID struct, this is
	// information about where this UID came from, and is unrelated to the
	// information about the resource we're matching. That data which is
	// used in the IFF function, is what you see in the struct fields here.
	BaseUID
	name string // the network interface name
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *NetUID) IFF(uid ResUID) bool {
	res, ok := uid.(*NetUID)
	if !ok {
		return false
	}
	return obj.name == res.name
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one although some resources can return multiple.
func (obj *NetRes) UIDs() []ResUID {
	x := &NetUID{
		BaseUID: BaseUID{Name: obj.GetName(), Kind: obj.GetKind()},
		name:    obj.Name,
	}
	return []ResUID{x}
}

// GroupCmp returns whether two resources can be grouped together or not.
func (obj *NetRes) GroupCmp(r Res) bool {
	_, ok := r.(*NetRes)
	if !ok {
		return false
	}
	return false
}

// Compare two resources and return if they are equivalent.
func (obj *NetRes) Compare(r Res) bool {
	// we can only compare NetRes to others of the same resource kind
	res, ok := r.(*NetRes)
	if !ok {
		return false
	}
	if !obj.BaseRes.Compare(res) {
		return false
	}
	if obj.Name != res.Name {
		return false
	}
	if obj.State != res.State {
		return false
	}
	if (obj.Addrs == nil) != (res.Addrs == nil) {
		return false
	}
	if err := StrSortedSliceCompare(obj.Addrs, res.Addrs); err != nil {
		return false
	}
	if obj.Gateway != res.Gateway {
		return false
	}

	return true
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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

// unitFileContents builds the unit file contents from the definition.
func (obj *NetRes) unitFileContents() []byte {
	// build the unit file contents
	u := []string{"[Match]"}
	u = append(u, fmt.Sprintf("Name=%s", obj.GetName()))
	u = append(u, "[Network]")
	for _, addr := range obj.Addrs {
		u = append(u, fmt.Sprintf("Address=%s", addr))
	}
	if obj.Gateway != "" {
		u = append(u, fmt.Sprintf("Gateway=%s", obj.Gateway))
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
		return ifaceDown, nil
	}
	// otherwise it's up
	return ifaceUp, nil
}

// linkUpDown brings the interface up or down, depending on input value.
func (obj *iface) linkUpDown(state string) error {
	if state != ifaceUp && state != ifaceDown {
		return fmt.Errorf("state must be up or down")
	}
	if state == ifaceUp {
		return netlink.LinkSetUp(obj.link)
	}
	return netlink.LinkSetDown(obj.link)
}

// getAddrs returns a list of strings containing all of the interface's
// IP addresses in CIDR format.
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
	for _, addr := range addrs {
		routeOK = false
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			return false, errwrap.Wrapf(err, "error parsing addr: %s", addr)
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
	for _, addr := range addrs {
		ip, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			return errwrap.Wrapf(err, "error parsing addr: %s", addr)
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

// addrApplyDelete, checks the interface's addresses and deletes any that are not
// in the list/definition.
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

// socketSet is used to receive events from a socket and shut it down cleanly
// when asked. It contains a socket for events and a pipe socket to unblock
// receive on shutdown.
type socketSet struct {
	fdEvents int
	fdPipe   int
	pipeFile string
}

// newSocketSet returns a socketSet, initialized with the given parameters.
func newSocketSet(groups uint32, file string) (*socketSet, error) {
	// make a netlink socket file descriptor
	fdEvents, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating netlink socket")
	}
	// bind to the socket and add add the netlink groups we need to get events
	if err := unix.Bind(fdEvents, &unix.SockaddrNetlink{
		Family: unix.AF_NETLINK,
		Groups: groups,
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding netlink socket")
	}

	// create a pipe socket to unblock unix.Select when we close
	fdPipe, err := unix.Socket(unix.AF_UNIX, unix.SOCK_RAW, unix.PROT_NONE)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error creating pipe socket")
	}
	// bind the pipe to a file
	if err = unix.Bind(fdPipe, &unix.SockaddrUnix{
		Name: file,
	}); err != nil {
		return nil, errwrap.Wrapf(err, "error binding pipe socket")
	}
	return &socketSet{
		fdEvents: fdEvents,
		fdPipe:   fdPipe,
		pipeFile: file,
	}, nil
}

// shutdown closes the event file descriptor and unblocks receive by sending
// a message to the pipe file descriptor. It must be called before close, and
// should only be called once.
func (obj *socketSet) shutdown() error {
	// close the event socket so no more events are produced
	if err := unix.Close(obj.fdEvents); err != nil {
		return err
	}
	// send a message to the pipe to unblock select
	return unix.Sendto(obj.fdPipe, nil, 0, &unix.SockaddrUnix{
		Name: path.Join(obj.pipeFile),
	})
}

// close closes the pipe file descriptor. It must only be called after
// shutdown has closed fdEvents, and unblocked receive. It should only be
// called once.
func (obj *socketSet) close() error {
	return unix.Close(obj.fdPipe)
}

// receive waits for bytes from fdEvents and parses them into a slice of
// netlink messages. It will block until an event is produced, or shutdown
// is called.
func (obj *socketSet) receive() ([]syscall.NetlinkMessage, error) {
	// Select will return when any fd in fdSet (fdEvents and fdPipe) is ready
	// to read.
	_, err := unix.Select(obj.nfd(), obj.fdSet(), nil, nil, nil)
	if err != nil {
		// if a system interrupt is caught
		if err == unix.EINTR { // signal interrupt
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error selecting on fd")
	}
	// receive the message from the netlink socket into b
	b := make([]byte, os.Getpagesize())
	n, _, err := unix.Recvfrom(obj.fdEvents, b, unix.MSG_DONTWAIT) // non-blocking receive
	if err != nil {
		// if fdEvents is closed
		if err == unix.EBADF { // bad file descriptor
			return nil, nil
		}
		return nil, errwrap.Wrapf(err, "error receiving messages")
	}
	// if we didn't get enough bytes for a header, something went wrong
	if n < unix.NLMSG_HDRLEN {
		return nil, fmt.Errorf("received short header")
	}
	b = b[:n] // truncate b to message length
	// use syscall to parse, as func does not exist in x/sys/unix
	return syscall.ParseNetlinkMessage(b)
}

// nfd returns one more than the highest fd value in the struct, for use as as
// the nfds parameter in select. It represents the file descriptor set maximum
// size. See man select for more info.
func (obj *socketSet) nfd() int {
	if obj.fdEvents > obj.fdPipe {
		return obj.fdEvents + 1
	}
	return obj.fdPipe + 1
}

// fdSet returns a bitmask representation of the integer values of fdEvents
// and fdPipe. See man select for more info.
func (obj *socketSet) fdSet() *unix.FdSet {
	fdSet := &unix.FdSet{}
	fdSet.Bits[obj.fdEvents/64] |= 1 << uint(obj.fdEvents)
	fdSet.Bits[obj.fdPipe/64] |= 1 << uint(obj.fdPipe) // fd = 3 becomes 100 if we add 5, we get 10100
	return fdSet
}

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
	"sort"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/util"

	systemd "github.com/coreos/go-systemd/dbus" // change namespace
	systemdUtil "github.com/coreos/go-systemd/util"
	"github.com/godbus/dbus"
	errwrap "github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

const (
	// IfacePrefix is used to identify unit files for managed links.
	IfacePrefix = "mgmt-"
	// ifaceUp is the up interface state
	ifaceUp = "up"
	// ifaceDown is the down interface state
	ifaceDown = "down"

	// dBusInterface is dBus's own dBus interface which contains methods and
	// additional interfaces used throughout the resource.
	dBusInterface = "org.freedesktop.DBus"
	// dBusAddMatch is the dBus method to filter signals when getting events by
	// eavesdropping on a given dBus interface.
	dBusAddMatch = dBusInterface + ".AddMatch"
	// dBusPropertiesInterface is the dBus interface used to get events when
	// the systemd unit properties change.
	dBusPropertiesInterface = dBusInterface + ".Properties"
	// dBusPropertiesGet is the dBus method to get a property from the provided
	// systemd unit.
	dBusPropertiesGet = dBusPropertiesInterface + ".Get"
	// dBusNetworkdInterface is the networkd dbus interface used to interact
	// with systemd-networkd units.
	dBusNetworkdInterface = "org.freedesktop.network1"
	// dBusNetworkdLinkInterface is the systemd-networkd interface used to
	// interact with network links over dBus.
	dBusNetworkdLinkInterface = dBusNetworkdInterface + ".Link"
	// dBusLinkPath is the dBus source path for network interface events.
	dBusLinkPath = "/org/freedesktop/network1/link/"
	// dBusEavesdropSprint is the format of dbus eavesdrop arguments.
	dBusEavesdropSprint = "type='signal', interface='%s', path='%s', eavesdrop='true'"
	// dBusFailMode is the dbus mode used for restarting the network service
	// that forces an error if the requested operation cannot be completed.
	dBusFailMode = "fail"

	// networkdUnitFileDir is the location of networkd unit files which define
	// the systemd network connections.
	networkdUnitFileDir = "/etc/systemd/network/"
	// networkdUnitFileExt is the file extension for networkd unit files.
	networkdUnitFileExt = ".network"
	// networkdService is the name of the networkd service.
	networkdService = "systemd-networkd.service"

	// activeState is the name of the dbus property for the systemd-networkd
	// service's "active state." It is used as a source for events and to check
	// the status of the service.
	activeState = "ActiveState"
	// activeStateActive is the active service state.
	activeStateActive = "active"
	// activeStateReloading is the reloading service state.
	activeStateReloading = "reloading"

	// opState is the name of the dbus property for the network interface's
	// "operational state." It is used and to check the status of the network
	// connection.
	opState = "OperationalState"
	// opStateRoutable us the routable operational state.
	opStateRoutable = "routable"
	// opStateNoCarrier is the no-carrier operational state.
	opStateNoCarrier = "no-carrier"
	// opStateOff is the off operational state.
	opStateOff = "off"

	// adminState is the name of the dbus property for the network interface's
	// "administrative state." It is used and to check if the interface is
	// configured or in a transitional state.
	adminState = "AdministrativeState"
	// adminStateConfiguring is the administrative state of the interface when
	// it is in the process of configuring.
	adminStateConfiguring = "configuring"

	// eventSources is the number of event sources in watch.
	eventSources = 5
)

func init() {
	RegisterResource("net", func() Res { return &NetRes{} })
}

// NetRes is an systemd-networkd resource.
type NetRes struct {
	BaseRes `yaml:",inline"`
	State   string   `yaml:"state"`   // up or down or empty
	DHCP    bool     `yaml:"dhcp"`    // enable dhcp
	Addrs   []string `yaml:"addrs"`   // list of addresses in cidr format
	Gateway string   `yaml:"gateway"` // gateway address

	ifacePath    dbus.ObjectPath // the dbus path to the interface
	unitFilePath string          // the interface unit file path

	// internal busses
	sdbus *systemd.Conn // go-systemd dbus connection
	gdbus *dbus.Conn    // godbus dbus connection
}

// Default returns some sensible defaults for this resource.
func (obj *NetRes) Default() Res {
	return &NetRes{
		BaseRes: BaseRes{
			MetaParams: DefaultMetaParams, // force a default
		},
		State: ifaceUp,
	}
}

// Validate if the params passed in are valid data.
func (obj *NetRes) Validate() error {
	// validate state
	if obj.State != ifaceUp && obj.State != ifaceDown && obj.State != "" {
		return fmt.Errorf("invalid state: %s", obj.State)
	}

	// validate dhcp
	if obj.DHCP && obj.Addrs != nil {
		return fmt.Errorf("cannot assign IP with DHCP enabled")
	}

	// validate network address input
	if obj.Addrs != nil {
		for _, addr := range obj.Addrs {
			if _, _, err := net.ParseCIDR(addr); err != nil {
				return errwrap.Wrapf(err, "error parsing address: %s", addr)
			}
		}
	}

	if obj.Gateway != "" {
		if g := net.ParseIP(obj.Gateway); g == nil {
			return fmt.Errorf("could not parse gateway: %s", obj.Gateway)
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
	// get the network link's dbus path
	if obj.ifacePath, err = dbusIfacePath(obj.GetName()); err != nil {
		return errwrap.Wrapf(err, "error getting iface dbus path")
	}

	// establish a go-systemd dbus connection
	obj.sdbus, err = systemd.New()
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to systemd")
	}
	// establish a coreos godbus dbus connection
	obj.gdbus, err = util.SystemBusPrivateUsable()
	if err != nil {
		return errwrap.Wrap(err, "error establishing system bus")
	}

	return obj.BaseRes.Init()
}

// Watch listens for networkd events by eavesdropping on the DBus Properties
// interface (org.freedesktop.DBus.Properties) and filtering for signals from
// the network device. It also watches the networkd service via a dbus unit
// subscription, and the unit file responsible for configuring the interface.
func (obj *NetRes) Watch() error {
	var err error
	// make sure systemd is running
	if !systemdUtil.IsRunningSystemd() {
		return fmt.Errorf("systemd is not running")
	}

	// create a dbus subscription to watch the networkd service unit
	set := obj.sdbus.NewSubscriptionSet()
	svcChan, svcErr := set.Subscribe() // svc event channel
	set.Add(networkdService)

	// Add a DBus rule to watch the "PropertiesChanged" signal and get messages
	// from the network link's dbus path.
	args := fmt.Sprintf(dBusEavesdropSprint, dBusPropertiesInterface, obj.ifacePath)
	if call := obj.gdbus.BusObject().Call(dBusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(err, "error creating dbus call")
	}

	// XXX: verify that implementation doesn't deadlock if there are unread
	// messages left in the channel
	ifaceChan := make(chan *dbus.Signal, 10) // iface event channel
	defer close(ifaceChan)

	obj.gdbus.Signal(ifaceChan)
	defer obj.gdbus.RemoveSignal(ifaceChan)

	// build the path to the configuration file
	obj.unitFilePath = networkdUnitFileDir + IfacePrefix + obj.GetName() + networkdUnitFileExt

	// watch the configuration file
	recWatcher, err := recwatch.NewRecWatcher(obj.unitFilePath, false)
	if err != nil {
		return err
	}
	defer recWatcher.Close()

	// notify engine that we're running
	if err := obj.Running(); err != nil {
		return err // bubble up a NACK...
	}

	var send = false
	var exit *error

	// closeMap keeps track of which event sources have closed their channels
	closeMap := make(map[int]bool)
	for i := 0; i < eventSources; i++ {
		closeMap[i] = false // initialize values
	}

	for {
		// check if each event source is closed
		closed := true
		for _, v := range closeMap {
			if !v {
				closed = false // something's not done
				break
			}
		}
		// if all are closed, we're done
		if closed {
			return nil
		}

		// process events
		select {
		case event, ok := <-ifaceChan:
			if !ok {
				closeMap[0] = true // done
				continue
			}
			if obj.debug {
				log.Printf("%s: Event: %v", obj, event)
			}

			send = true
			obj.StateOK(false) // dirty

		case event, ok := <-recWatcher.Events():
			if !ok {
				closeMap[1] = true // done
				continue
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "Unknown %s watcher error", obj)
			}
			if obj.debug {
				log.Printf("%s: Event(%s): %v", obj, event.Body.Name, event.Body.Op)
			}

			send = true
			obj.StateOK(false) // dirty

		case event, ok := <-svcChan:
			if !ok {
				closeMap[2] = true // done
				continue
			}
			if obj.debug {
				log.Printf("%s: Event(%s): %v", obj, event[networkdService].ActiveState, event)
			}

			send = true
			obj.StateOK(false) // dirty

		case err, ok := <-svcErr:
			if !ok {
				closeMap[3] = true // done
				continue
			}
			return errwrap.Wrapf(err, "unknown %s error", obj)

		case event, ok := <-obj.Events():
			if !ok {
				closeMap[4] = true // done
				continue
			}
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

// ifaceCheckApply checks the state of the network device and performs the
// appropriate actions to converge its state with the graph.
func (obj *NetRes) ifaceCheckApply(apply bool) (bool, error) {
	// create a bus object pointing to the adapter
	busObj := obj.gdbus.Object(dBusNetworkdInterface, obj.ifacePath)
	// get the object's operational state
	call := busObj.Call(dBusPropertiesGet, 0, dBusNetworkdLinkInterface, opState)
	if call.Err != nil {
		return false, call.Err
	}
	// check the operational state
	os, ok := call.Body[0].(dbus.Variant)
	if !ok {
		return false, fmt.Errorf("error casting opstate")
	}

	// if the interface's state is correct, we're done
	if obj.State == ifaceDown && os == dbus.MakeVariant(opStateOff) {
		return true, nil
	}
	if obj.State == ifaceUp {
		if os != dbus.MakeVariant(opStateOff) {
			return true, nil
		}
		// XXX: retry logic for fault tolerance
		if os == dbus.MakeVariant(opStateNoCarrier) {
			return false, fmt.Errorf("connection failed")
		}
	}

	// if the service is restarting, wait for the next event
	restart, err := obj.svcRestarting()
	if err != nil {
		return false, err
	}
	if restart {
		return false, nil
	}

	if !apply {
		return false, nil
	}

	log.Printf("%s: ifaceCheckApply(%t)", obj, apply)

	link, err := netlink.LinkByName(obj.GetName())
	if err != nil {
		return false, errwrap.Wrapf(err, "error finding link by name")
	}

	// enable or disable the interface
	if obj.State == ifaceUp {
		netlink.LinkSetUp(link)
	}
	if obj.State == ifaceDown {
		netlink.LinkSetDown(link)
	}
	return false, nil
}

// fileCheckApply checks and maintains the systemd-networkd unit file contents.
// TODO: check for conflicting unit files and think about unit priority
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
	if err := ioutil.WriteFile(obj.unitFilePath, contents, 0644); err != nil {
		return false, errwrap.Wrapf(err, "error writing configuration file")
	}
	return false, nil
}

// ipCheckApply checks if the interface has the correct addresses and restarts
// the networkd service if the addresses are inconsistent.
func (obj *NetRes) ipCheckApply(apply bool) (bool, error) {
	var ifaceAddrs []string

	if obj.Addrs == nil {
		return true, nil
	}

	// if the interface is configuring, wait for the next event
	// create a bus object pointing to the adapter
	busObj := obj.gdbus.Object(dBusNetworkdInterface, obj.ifacePath)
	// get the object's operational state
	call := busObj.Call(dBusPropertiesGet, 0, dBusNetworkdLinkInterface, adminState)
	if call.Err != nil {
		return false, call.Err
	}
	// check the operational state
	as, ok := call.Body[0].(dbus.Variant)
	if !ok {
		return false, fmt.Errorf("error casting opstate")
	}
	if as == dbus.MakeVariant(adminStateConfiguring) {
		return false, nil
	}

	// find the specified interface and get a list of addresses
	iface, err := net.InterfaceByName(obj.GetName())
	if err != nil {
		return false, errwrap.Wrapf(err, "error finding interface: %s", obj.GetName())
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return false, errwrap.Wrapf(err, "error getting addrs from interface: %s", obj.GetName())
	}
	// if the address is not a link local address, add it to the list
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			return false, errwrap.Wrapf(err, "error parsing interface addr: %s", addr)
		}
		// XXX: ignore multicast too?
		if !ip.IsLinkLocalUnicast() {
			ifaceAddrs = append(ifaceAddrs, addr.String())
		}
	}

	// compare defined addrs with actual addrs
	if strSliceCompare(obj.Addrs, ifaceAddrs) {
		return true, nil
	}
	// end of state checking
	if !apply {
		return false, nil
	}

	log.Printf("%s: ipCheckApply(%t)", obj, apply)

	// restart networkd to re-initialize the interface
	if err := restartSystemdUnit(obj.sdbus, networkdService, dBusFailMode); err != nil {
		return false, errwrap.Wrapf(err, "error restarting networkd service")
	}

	return false, nil
}

// svcCheckApply checks the state of the networkd service and restarts it, if
// necessary. This method checks if ipCheckApply already restarted networkd
// so we don't restart it again unnecessarily.
func (obj *NetRes) svcCheckApply(apply bool) (bool, error) {
	// make sure systemd is running
	if !systemdUtil.IsRunningSystemd() {
		return false, fmt.Errorf("systemd is not running")
	}

	// get the service's state
	svcState, err := obj.sdbus.GetUnitProperty(networkdService, activeState)
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get svc unit properties")
	}
	// if it's running, we're done
	if svcState.Value == dbus.MakeVariant(activeStateActive) {
		return true, nil
	}
	// if the service is restarting, there's nothing to do
	if svcState.Value == dbus.MakeVariant(activeStateReloading) {
		return false, nil
	}

	if !apply {
		return false, nil
	}

	log.Printf("%s: svcCheckApply(%t)", obj, apply)

	// restart networkd
	if err := restartSystemdUnit(obj.sdbus, networkdService, dBusFailMode); err != nil {
		return false, errwrap.Wrapf(err, "error restarting networkd service")
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

	// check the unit file
	if c, err := obj.fileCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// check the networkd service
	if c, err := obj.svcCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	// check the ip addresses
	if c, err := obj.ipCheckApply(apply); err != nil {
		return false, err
	} else if !c {
		checkOK = false
	}

	return checkOK, nil
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
	if obj.DHCP != res.DHCP {
		return false
	}
	if (obj.Addrs == nil) != (res.Addrs == nil) {
		return false
	}
	if obj.Addrs != nil && !strSliceCompare(obj.Addrs, res.Addrs) {
		return false
	}
	if obj.Gateway != res.Gateway {
		return false
	}

	return true
}

// Close cleans up some resources so we can exit cleanly.
func (obj *NetRes) Close() error {
	// close the dbus connection
	obj.sdbus.Close() // returns nothing
	// close the message bus
	if err := obj.gdbus.Close(); err != nil {
		return errwrap.Wrapf(err, "error closing godbus message bus")
	}

	return obj.BaseRes.Close()
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

// unitFileContents builds the unit file contents from the input
func (obj *NetRes) unitFileContents() []byte {
	// build the unit file contents
	u := []string{"[Match]"}
	u = append(u, fmt.Sprintf("Name=%s", obj.GetName()))
	u = append(u, "[Network]")
	if obj.DHCP {
		u = append(u, "DHCP=yes")
	}
	for _, addr := range obj.Addrs {
		u = append(u, fmt.Sprintf("Address=%s", addr))
	}
	if obj.Gateway != "" {
		u = append(u, fmt.Sprintf("Gateway=%s", obj.Gateway))
	}
	c := strings.Join(u, "\n")
	return []byte(c)
}

// getAddrsFromIface returns the provided interface's IPv4 and IPv6 addresses.
// If an address isn't found, an empty string is returned in its place.
func getAddrsFromIface(name string) ([]net.Addr, error) {
	// find the interface
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return nil, errwrap.Wrapf(err, "error finding interface: %s", name)
	}

	// check the interface's ip addresses
	return iface.Addrs()
}

// restartSystemdUnit restarts the specified systemd unit.
func restartSystemdUnit(conn *systemd.Conn, unit, mode string) error {
	// connect to systemd
	connChan := make(chan string) // catch result information
	defer close(connChan)

	// restart unit
	if _, err := conn.RestartUnit(unit, mode, connChan); err != nil {
		return errwrap.Wrapf(err, "failed to restart unit")
	}

	// XXX: will this ever hang?
	select {
	case msg, ok := <-connChan:
		if !ok {
			return nil
		}
		if msg != "done" {
			return fmt.Errorf("unexpected systemd return string: %s", msg)
		}
	}
	return nil
}

// svcRestarting returns true if the systemd-networkd service is currently
// restarting and false if it is not.
// XXX: should this take the params as arguments to be more general like
// restartSystemdUnit?
func (obj *NetRes) svcRestarting() (bool, error) {
	// get the networkd service state
	svcState, err := obj.sdbus.GetUnitProperty(networkdService, activeState)
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to get svc unit properties")
	}
	if svcState.Value == dbus.MakeVariant(activeStateReloading) {
		return true, nil
	}
	return false, nil
}

// dbusIfacePath takes a device name and returns the corresponding dbus path.
func dbusIfacePath(name string) (dbus.ObjectPath, error) {
	// find the interface
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", errwrap.Wrapf(err, "error finding interface: %s", name)
	}
	// Get the node path for the interface by parsing the index. The first
	// digit is converted to an ascii value, while subsequent digits are
	// preserved ie 1 becomes 31, 15 becomes 315, etc. For more info, see:
	// https://lists.freedesktop.org/archives/systemd-devel/2016-May/036531.html
	s := strconv.Itoa(iface.Index)
	r := fmt.Sprintf("%x", s[0])
	i := r + s[1:]
	return dbus.ObjectPath(dBusLinkPath + fmt.Sprintf("_%s", i)), nil
}

// strSliceCompare takes two lists of strings and returns whether or not they
// are identical
func strSliceCompare(x, y []string) bool {
	if len(x) != len(y) {
		return false
	}
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

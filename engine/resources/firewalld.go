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

package resources

import (
	"context"
	"fmt"
	"os/user"
	"strconv"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
	"github.com/google/nftables"
)

func init() {
	// TODO: Should we split this into firewalld:service and firewalld:port?
	engine.RegisterResource(KindFirewalld, func() engine.Res { return &FirewalldRes{} })

	// const.res.firewalld.state.exists = "exists"
	// const.res.firewalld.state.absent = "absent"
	vars.RegisterResourceParams(KindFirewalld, map[string]map[string]func() interfaces.Var{
		ParamFileState: {
			FirewalldStateExists: func() interfaces.Var {
				return &types.StrValue{
					V: FirewalldStateExists,
				}
			},
			FirewalldStateAbsent: func() interfaces.Var {
				return &types.StrValue{
					V: FirewalldStateAbsent,
				}
			},
			// TODO: consider removing this field entirely
			"undefined": func() interfaces.Var {
				return &types.StrValue{
					V: FirewalldStateUndefined, // empty string
				}
			},
		},
	})
}

const (
	// KindFirewalld is the kind string used to identify this resource.
	KindFirewalld = "firewalld"

	// ParamFirewalldState is the name of the state field parameter.
	ParamFirewalldState = "state"

	// FirewalldStateExists is the string that represents that the service,
	// or port, or other setting should be present.
	FirewalldStateExists = "exists"

	// FirewalldStateAbsent is the string that represents that the service,
	// or port, or other setting should not exist.
	FirewalldStateAbsent = "absent"

	// FirewalldStateUndefined means the file state has not been specified.
	// TODO: consider moving to *string and express this state as a nil.
	FirewalldStateUndefined = ""

	// ErrInvalidZone is a firewalld error from dbus.
	ErrInvalidZone = engine.Error("invalid zone")

	// ErrInvalidPort is a firewalld error from dbus.
	ErrInvalidPort = engine.Error("invalid port")

	// ErrInvalidService is a firewalld error from dbus.
	ErrInvalidService = engine.Error("invalid service")

	// ErrInvalidProtocol is a firewalld error from dbus.
	ErrInvalidProtocol = engine.Error("invalid protocol")

	// ErrInvalidCommand is a firewalld error from dbus.
	ErrInvalidCommand = engine.Error("invalid command")

	// ErrMissingProtocol is a firewalld error from dbus.
	ErrMissingProtocol = engine.Error("missing protocol")

	// ErrAlreadyEnabled is a firewalld error from dbus.
	ErrAlreadyEnabled = engine.Error("already enabled")

	// ErrNotEnabled is a firewalld error from dbus.
	ErrNotEnabled = engine.Error("not enabled")

	firewalld1Path  = dbus.ObjectPath("/org/fedoraproject/FirewallD1")
	firewalld1Iface = "org.fedoraproject.FirewallD1"
)

// FirewalldRes is a simple resource to interact with the firewalld service. It
// is not a replacement for a modern, robust tool like `shorewall`, but it has
// its uses such as for small, desktop use cases. The API of this resource might
// change to either add new features, split this into multiple resources, or to
// optimize the execution if it turns out to be too expensive to run large
// amounts of these as-is. The name variable currently has no useful purpose.
// Keep in mind that this resource requires root permissions to be able change
// the firewall settings and to monitor for changes. The change detection uses
// the nftables monitor facility.
type FirewalldRes struct {
	traits.Base // add the base methods without re-implementation

	// XXX: add traits.Reversible and make this undo itself on removal

	init *engine.Init

	// Zone is the name of the zone to manage. If unspecified, we will
	// attempt to get the default zone automatically. In this situation, it
	// is possible that this default changes over time if it is acted upon
	// by external tools that use firewalld.
	Zone string `lang:"zone" yaml:"zone"`

	// State is the desired state.
	State string `lang:"state" yaml:"state"`

	// Services are the list of services to manage to the desired state.
	// These are single lower case strings like `dhcp`, and `tftp`.
	Services []string `lang:"services" yaml:"services"`

	// Ports are the list of port/protocol combinations to manage to the
	// desired state. These are strings of port number (slash) protocol like
	// `4280/tcp` and `38/udp`.
	Ports []string `lang:"ports" yaml:"ports"`

	// TODO: add a boolean setting for persistence (across reboots)
	// TODO: this is the equivalent of `firewall-cmd --permanent`
	//Permanent bool `lang:"permanent" yaml:"permanent"`

	zone string // cached name of the zone we're managing
	call callFunc
	wg   *sync.WaitGroup
}

// Default returns some sensible defaults for this resource.
func (obj *FirewalldRes) Default() engine.Res {
	return &FirewalldRes{}
}

// Validate if the params passed in are valid data.
func (obj *FirewalldRes) Validate() error {
	// Check that we're running as root or with enough capabilities.
	// XXX: check that we have enough capabilities if we're not root
	currentUser, err := user.Current()
	if err != nil {
		return errwrap.Wrapf(err, "error looking up current user")
	}
	if currentUser.Uid != "0" {
		// TODO: Technically we only need perms to watch state and to
		// change things, so without this we could just error entirely
		// if this were only used as a change detection tool.
		return fmt.Errorf("running as root is required for firewalld")
	}

	if obj.State != "" {
		if obj.State != FirewalldStateExists && obj.State != FirewalldStateAbsent {
			return fmt.Errorf("invalid state: %s", obj.State)
		}
	}

	for _, x := range obj.Services {
		// TODO: we could check services from a list
		if x == "" {
			return fmt.Errorf("service is empty")
		}
	}

	for _, x := range obj.Ports {
		split := strings.Split(x, "/")
		if len(split) != 2 {
			return fmt.Errorf("port/protocol was invalid: %s", x)
		}

		if num, err := strconv.Atoi(split[0]); err != nil {
			return err
		} else if num <= 0 {
			return fmt.Errorf("invalid number: %d", num)
		}

		// TODO: we could check protocols from a list
		if split[1] == "" {
			return fmt.Errorf("protocol is empty")
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *FirewalldRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.wg = &sync.WaitGroup{}

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *FirewalldRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events. This
// could have been implemented with an exec of `/usr/sbin/nft --json monitor`,
// but the wrapped polkit and difficulty getting permissions using capabilities
// and without using full root, was challenging. This is cleaner too.
func (obj *FirewalldRes) Watch(ctx context.Context) error {
	defer obj.wg.Wait()

	// `sudo setcap CAP_NET_ADMIN=+eip mgmt` seems to let us avoid root
	opts := []nftables.ConnOption{}
	conn, err := nftables.New(opts...) // (*nftables.Conn, error)
	if err != nil {
		return err
	}

	// XXX: filter out events we don't care about using WithMonitorObject
	//opt := nftables.WithMonitorObject(???)
	mopts := []nftables.MonitorOption{}
	//mopts = append(mopts, opt)
	monitor := nftables.NewMonitor(mopts...) // *nftables.Monitor
	defer monitor.Close()
	events, err := conn.AddMonitor(monitor)
	if err != nil {
		return err
	}

	obj.init.Running() // when started, notify engine that we're running

	for {
		select {
		case event, ok := <-events: // &nftables.MonitorEvent
			if !ok {
				return nil
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "monitor err")
			}
			if obj.init.Debug {
				//obj.init.Logf("event: %#v", event)
				obj.init.Logf("event type: %v", event.Type)
				//obj.init.Logf("event data: %+v", event.Data)
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply checks the resource state and applies the resource if the bool
// input is true. It returns error info and if the state check passed or not.
func (obj *FirewalldRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	conn, err := util.SystemBusPrivateUsable()
	if err != nil {
		return false, errwrap.Wrapf(err, "failed to connect to the private system bus")
	}
	defer conn.Close()

	firewalldObject := conn.Object(firewalld1Iface, firewalld1Path) // TODO: can we reuse this?
	var flags dbus.Flags                                            // none set
	obj.call = func(ctx context.Context, method string, args ...interface{}) *dbus.Call {
		fullMethod := firewalld1Iface + method // eg: ".getDefaultZone"
		return firewalldObject.CallWithContext(ctx, fullMethod, flags, args...)
	}

	// NOTE: The default zone might change over time by external users...
	obj.zone = obj.Zone // default to this
	if obj.Zone == "" {
		if err := obj.call(ctx, ".getDefaultZone").Store(&obj.zone); err != nil {
			return false, err
		}
		if obj.zone == "" {
			return false, fmt.Errorf("unexpected empty zone")
		}
		if obj.init.Debug {
			obj.init.Logf("zone: %s", obj.zone)
		}
	}

	checkOK := true

	// This ordering doesn't currently matter, but might change if we find
	// any sort of relevant relationship.
	for _, x := range obj.Services {
		if c, err := obj.serviceCheckApply(ctx, apply, x); err != nil {
			return false, err
		} else if !c {
			checkOK = false
		}
	}

	for _, x := range obj.Ports {
		if c, err := obj.portCheckApply(ctx, apply, x); err != nil {
			return false, err
		} else if !c {
			checkOK = false
		}
	}

	return checkOK, nil
}

// does the equivalent of: `firewall-cmd --zone=<zone> --list-services` and:
// `firewall-cmd --zone=<zone> --add-service=<service>` and: `firewall-cmd
// --zone=<zone> --remove-service=<service>`.
func (obj *FirewalldRes) serviceCheckApply(ctx context.Context, apply bool, service string) (bool, error) {
	// .zone.getServices(s: zone) -> as
	var services []string
	args := []interface{}{obj.zone}
	if err := obj.call(ctx, ".zone.getServices", args...).Store(&services); err != nil {
		if parseError(err) == ErrInvalidZone {
			obj.init.Logf("did the zone change?") // two managers!
		}
		return false, err
	}
	if obj.init.Debug {
		obj.init.Logf("services: %+v", services)
	}

	found := util.StrInList(service, services)
	exists := obj.State != FirewalldStateAbsent // other states mean "exists"

	if exists && found || !exists && !found {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if !found {
		// add it
		// .zone.addService(s: zone, s: service, i: timeout) -> s
		timeout := 0 // TODO: what should this be?
		var output string
		addArgs := []interface{}{obj.zone, service, timeout}
		if err := obj.call(ctx, ".zone.addService", addArgs...).Store(&output); err != nil && parseError(err) != ErrAlreadyEnabled {
			return false, err
		}
		obj.init.Logf("service added: %s", service)
		if output != obj.zone {
			// programming error
			return false, fmt.Errorf("dbus API inconsistency")
		}

		return false, nil
	}

	// remove it
	// .zone.removeService(s: zone, s: service) -> s
	//timeout := 0
	var output string
	removeArgs := []interface{}{obj.zone, service}
	if err := obj.call(ctx, ".zone.removeService", removeArgs...).Store(&output); err != nil && parseError(err) != ErrNotEnabled {
		return false, err
	}
	obj.init.Logf("service removed: %s", service)
	if output != obj.zone {
		// programming error
		return false, fmt.Errorf("dbus API inconsistency")
	}

	return false, nil
}

// does the equivalent of: `firewall-cmd --zone=<zone> --list-ports` and:
// `firewall-cmd --zone=<zone> --add-port=4280/tcp` and: `firewall-cmd
// --zone=<zone> --remove-port=4280/tcp`.
func (obj *FirewalldRes) portCheckApply(ctx context.Context, apply bool, pp string) (bool, error) {
	split := strings.Split(pp, "/")
	if len(split) != 2 { // should already be checked in Validate
		return false, fmt.Errorf("port/protocol was invalid: %s", pp)
	}
	port, err := strconv.Atoi(split[0])
	if err != nil {
		return false, err
	} else if port <= 0 {
		return false, fmt.Errorf("invalid number: %d", port)
	}
	protocol := split[1]

	// .zone.getPorts(s: zone) -> aas
	var ports [][]string
	args := []interface{}{obj.zone}
	if err := obj.call(ctx, ".zone.getPorts", args...).Store(&ports); err != nil {
		if parseError(err) == ErrInvalidZone {
			obj.init.Logf("did the zone change?") // two managers!
		}
		return false, err
	}
	if obj.init.Debug {
		obj.init.Logf("ports: %+v", ports)
	}

	found := false
	for i, x := range ports {
		// eg: ["1025-65535", "udp"]
		if len(x) != 2 {
			return false, fmt.Errorf("unexpected ports length (%d), got: %v", len(x), x)
		}

		// x[0] is range or single value, eg: "1025-65535" OR "42"
		// x[1] is proto like "tcp" or "udp"
		if obj.init.Debug {
			obj.init.Logf("rule %d: %s/%s", i, x[0], x[1])
		}

		if protocol != x[1] {
			continue // not found (not us)
		}

		// does the port number match?
		split := strings.Split(x[0], "-") // split the range
		if len(split) != 1 && len(split) != 2 {
			return false, fmt.Errorf("unexpected ports format (%d), got: %v", len(split), split)
		}
		if len(split) == 1 { // standalone single value
			if num, err := strconv.Atoi(split[0]); err != nil {
				return false, err // programming error
			} else if num != port {
				continue // not found
			}
			found = true // yay!
			break
		}
		//if len(split) == 2
		lhs, err := strconv.Atoi(split[0])
		if err != nil {
			return false, err // programming error
		}
		rhs, err := strconv.Atoi(split[1])
		if err != nil {
			return false, err // programming error
		}

		if lhs <= port && port <= rhs { // ranges are inclusive on both boundss
			found = true // yay!
			break
		}
	}

	exists := obj.State != FirewalldStateAbsent // other states mean "exists"

	if exists && found || !exists && !found {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if !found {
		// add it
		// .zone.addPort(s: zone, s: port, s: protocol, i: timeout) -> s
		timeout := 0 // TODO: what should this be?
		var output string
		addArgs := []interface{}{obj.zone, strconv.Itoa(port), protocol, timeout}
		if err := obj.call(ctx, ".zone.addPort", addArgs...).Store(&output); err != nil && parseError(err) != ErrAlreadyEnabled {
			return false, err
		}
		obj.init.Logf("port added: %s", pp)
		if output != obj.zone {
			// programming error
			return false, fmt.Errorf("dbus API inconsistency")
		}

		return false, nil
	}

	// remove it
	// .zone.removePort(s: zone, s: port, s: protocol) -> s
	//timeout := 0
	var output string
	removeArgs := []interface{}{obj.zone, strconv.Itoa(port), protocol}
	if err := obj.call(ctx, ".zone.removePort", removeArgs...).Store(&output); err != nil && parseError(err) != ErrNotEnabled {
		return false, err
	}
	obj.init.Logf("port removed: %s", pp)
	if output != obj.zone {
		// programming error
		return false, fmt.Errorf("dbus API inconsistency")
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *FirewalldRes) Cmp(r engine.Res) error {
	// we can only compare FirewalldRes to others of the same resource kind
	res, ok := r.(*FirewalldRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Zone != res.Zone {
		return fmt.Errorf("the Zone differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	if len(obj.Services) != len(res.Services) {
		return fmt.Errorf("the Services differ")
	}
	for i, x := range obj.Services {
		if x != res.Services[i] {
			return fmt.Errorf("the service at index %d differs", i)
		}
	}

	if len(obj.Ports) != len(res.Ports) {
		return fmt.Errorf("the Ports differ")
	}
	for i, x := range obj.Ports {
		if x != res.Ports[i] {
			return fmt.Errorf("the port/protocol at index %d differs", i)
		}
	}

	// TODO: consider adding this setting
	//if obj.Permanent != res.Permanent {
	//	return fmt.Errorf("the Permanent differs")
	//}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *FirewalldRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes FirewalldRes // indirection to avoid infinite recursion

	def := obj.Default()           // get the default
	res, ok := def.(*FirewalldRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to FirewalldRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = FirewalldRes(raw) // restore from indirection with type conversion!
	return nil
}

// callFunc is a helper func for simplifying the making of dbus calls.
type callFunc func(ctx context.Context, method string, args ...interface{}) *dbus.Call

// parseError converts a returned error into one of our error constants if it
// matches. If it doesn't match, then it passes the data through. This is a
// useful alternative mechanism to inline string matching on the returned error.
func parseError(err error) error {
	if err == nil {
		return nil // passthrough
	}
	dbusError, ok := err.(dbus.Error)
	if !ok {
		return err // passthrough
	}

	if s := firewalld1Iface + ".Exception"; dbusError.Name != s {
		return err // passthrough
	}

	// eg:
	// dbus.Error{
	//	Name:"org.fedoraproject.FirewallD1",
	//	Body:[]interface {}{
	//		"NOT_ENABLED: '4280:tcp' not in 'FedoraWorkstation'",
	//	},
	//}
	if len(dbusError.Body) != 1 { // TODO: does this happen?
		return err // passthrough
	}

	body, ok := (dbusError.Body[0]).(string)
	if !ok {
		return err // passthrough
	}

	// eg: "NOT_ENABLED: '4280:tcp' not in 'FedoraWorkstation'"
	sep := ": " // the first colon space after the error name
	split := strings.Split(body, sep)
	//other := strings.Join(split[1:], sep) // remaining data (detailed reason)
	switch split[0] {
	case "INVALID_ZONE":
		return ErrInvalidZone
	case "INVALID_PORT":
		return ErrInvalidPort
	case "INVALID_SERVICE":
		return ErrInvalidService
	case "INVALID_PROTOCOL":
		return ErrInvalidProtocol
	case "INVALID_COMMAND":
		return ErrInvalidCommand
	case "MISSING_PROTOCOL":
		return ErrMissingProtocol
	case "ALREADY_ENABLED": // we tried to add it but it was already there
		return ErrAlreadyEnabled
	case "NOT_ENABLED": // we tried to remove it but it was already gone
		return ErrNotEnabled
	}

	return err // passthrough
}

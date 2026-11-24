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

// Package systemd contains small helpers for talking to systemd directly over
// D-Bus. Nothing in here polls.
//
// DOCS: https://www.freedesktop.org/software/systemd/man/latest/org.freedesktop.systemd1.html
package systemd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

const (
	// BusName is the well-known bus name that systemd owns.
	BusName = "org.freedesktop.systemd1"

	// ManagerPath is the object path of the systemd manager.
	ManagerPath = dbus.ObjectPath("/org/freedesktop/systemd1")

	// ManagerInterface is the interface of the systemd manager.
	ManagerInterface = "org.freedesktop.systemd1.Manager"

	// UnitInterface is the interface that every systemd unit implements.
	UnitInterface = "org.freedesktop.systemd1.Unit"

	// unitPathPrefix is the start of the object path of every unit.
	unitPathPrefix = "/org/freedesktop/systemd1/unit/"

	// alreadySubscribed is the error name systemd returns if we call
	// Subscribe on a connection that is already subscribed.
	alreadySubscribed = "org.freedesktop.systemd1.AlreadySubscribed"

	// The bus itself sends NameOwnerChanged from here.
	dbusName      = "org.freedesktop.DBus"
	dbusPath      = dbus.ObjectPath("/org/freedesktop/DBus")
	dbusInterface = "org.freedesktop.DBus"

	// propertiesInterface sends PropertiesChanged.
	propertiesInterface = "org.freedesktop.DBus.Properties"

	// signalBuffer is the size of our signal channel. If it fills up godbus
	// does not drop the signal, it parks it in a goroutine instead, so this
	// only needs to be big enough to absorb a normal burst.
	signalBuffer = 16
)

// UnitWatcher tells you when something about a systemd unit may have changed.
// It doesn't poll. It asks the bus for only the signals which concern this one
// unit, so while nothing changes, nothing is received. It doesn't tell you what
// the new state is, so read that yourself after each event. Its methods are not
// safe to call concurrently, but the Events channel can be used from anywhere.
type UnitWatcher struct {
	// Unit is the full name of the unit to watch, eg: sshd.service.
	Unit string

	// Session watches the systemd user instance on the session bus instead
	// of the system instance on the system bus.
	Session bool

	conn   *dbus.Conn
	path   dbus.ObjectPath
	events chan string
	err    error // why we stopped, set before events is closed
	cancel func()
	done   chan struct{} // closed once the goroutine has exited
}

// Init connects to the bus and subscribes to all the signals that we need. Any
// change that happens after this returns will be seen on Events, so once it has
// returned, it's safe to read the current state of the unit without racing a
// change that happens in between. The watcher runs until ctx is cancelled or
// Close is called. If this errors, there is nothing to Close.
func (obj *UnitWatcher) Init(ctx context.Context) error {
	if obj.Unit == "" {
		return fmt.Errorf("empty unit name")
	}
	path := UnitPath(obj.Unit)

	var conn *dbus.Conn
	var err error
	if obj.Session {
		conn, err = util.SessionBusPrivateUsable()
	} else {
		conn, err = util.SystemBusPrivateUsable()
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}

	// Register our channel before adding any match, so that we can't miss a
	// signal that arrives in between.
	ch := make(chan *dbus.Signal, signalBuffer)
	conn.Signal(ch)

	// This is a private connection, so the bus only sends us what these
	// rules match. We match the sender too, since anyone on the bus can
	// emit a signal with these names.
	//
	// We deliberately don't match UnitNew or UnitRemoved. Reading any
	// property of a unit which isn't loaded makes systemd load it, which
	// sends UnitNew, and then unload it again soon after, which sends
	// UnitRemoved. If we reported those, then every CheckApply that reads
	// the state of an idle unit would cause another event, forever. We
	// don't need them, since a real start or stop changes ActiveState.
	matches := [][]dbus.MatchOption{
		{ // the state of this one unit, eg: ActiveState
			dbus.WithMatchSender(BusName),
			dbus.WithMatchObjectPath(path),
			dbus.WithMatchInterface(propertiesInterface),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchArg(0, UnitInterface),
		},
		{ // a daemon-reload, which may have loaded a new unit file
			dbus.WithMatchSender(BusName),
			dbus.WithMatchObjectPath(ManagerPath),
			dbus.WithMatchInterface(ManagerInterface),
			dbus.WithMatchMember("Reloading"),
		},
		{ // an enable or disable, UnitFileState never signals a change
			dbus.WithMatchSender(BusName),
			dbus.WithMatchObjectPath(ManagerPath),
			dbus.WithMatchInterface(ManagerInterface),
			dbus.WithMatchMember("UnitFilesChanged"),
		},
		{ // systemd restarted, so we need to subscribe again
			dbus.WithMatchSender(dbusName),
			dbus.WithMatchObjectPath(dbusPath),
			dbus.WithMatchInterface(dbusInterface),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, BusName),
		},
	}
	for _, match := range matches {
		if err := conn.AddMatchSignalContext(ctx, match...); err != nil {
			_ = conn.Close()
			return errwrap.Wrapf(err, "failed to add match signal on bus")
		}
	}

	if err := subscribe(ctx, conn); err != nil {
		_ = conn.Close()
		return err
	}

	obj.conn = conn
	obj.path = path
	obj.start(ctx, ch)
	return nil
}

// Events returns a channel which receives a short description, which is useful
// for debugging, each time something about the unit may have changed. At most
// one event is ever waiting. If another change happens before you've received
// it, no second event is queued, because the waiting one already tells you to
// go and read the state, which will include both changes. This means a burst of
// changes only wakes you up once or twice. The channel is closed when the
// watcher stops, and then Err says why.
func (obj *UnitWatcher) Events() <-chan string {
	return obj.events
}

// Err returns the reason that the watcher stopped. It's never nil once Events
// has been closed, and you must not call it before then.
func (obj *UnitWatcher) Err() error {
	return obj.err
}

// start launches the goroutine that turns signals from ch into events. It never
// blocks on whoever reads the events, so we can always stop it by cancelling.
func (obj *UnitWatcher) start(ctx context.Context, ch <-chan *dbus.Signal) {
	ctx, cancel := context.WithCancel(ctx)
	obj.cancel = cancel
	obj.events = make(chan string, 1) // see Events for why only one
	obj.done = make(chan struct{})

	go func() {
		defer close(obj.done)
		obj.err = obj.run(ctx, ch)
		close(obj.events) // after err is set, so that Err is safe to call
	}()
}

// run is the body of the goroutine. It only returns when we have to stop, and
// that is always with an error which says why.
func (obj *UnitWatcher) run(ctx context.Context, ch <-chan *dbus.Signal) error {
	for {
		select {
		case sig, ok := <-ch:
			if !ok { // godbus closes this when the connection closes
				return fmt.Errorf("bus connection closed")
			}
			msg, err := obj.process(ctx, sig)
			if err != nil {
				return err
			}
			if msg == "" {
				continue
			}
			select {
			case obj.events <- msg:
			default: // one is already waiting, and it covers this one too
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// process looks at a single signal and returns a description of it if it's one
// we should report, or the empty string if it should be ignored. We also see
// some signals that we never matched, such as the NameAcquired that the bus
// sends to every new connection, so those are ignored here.
func (obj *UnitWatcher) process(ctx context.Context, sig *dbus.Signal) (string, error) {
	switch sig.Name {
	case propertiesInterface + ".PropertiesChanged":
		// The body is: interface, changed properties, invalidated.
		if sig.Path != obj.path || len(sig.Body) == 0 || sig.Body[0] != UnitInterface {
			return "", nil
		}
		if s := activeState(sig.Body); s != "" {
			return fmt.Sprintf("unit is %s", s), nil
		}
		return "unit changed", nil

	case ManagerInterface + ".Reloading":
		// This is sent with true when a reload starts, and with false
		// once it's done. Only the second one matters.
		if len(sig.Body) == 1 && sig.Body[0] == true {
			return "", nil
		}
		return "daemon reloaded", nil

	case ManagerInterface + ".UnitFilesChanged":
		return "unit files changed", nil

	case dbusInterface + ".NameOwnerChanged":
		// The body is: name, old owner, new owner.
		if len(sig.Body) != 3 || sig.Body[0] != BusName {
			return "", nil
		}
		if sig.Body[2] == "" {
			// systemd went away, eg: it's re-executing itself, or a
			// user instance stopped. There's nothing to ask for
			// now, so wait until it comes back.
			return "", nil
		}
		if err := subscribe(ctx, obj.conn); err != nil {
			return "", err
		}
		return "systemd restarted", nil
	}

	return "", nil
}

// Close stops the watcher, waits for its goroutine to exit, and then closes our
// bus connection. We do not remove our match rules or unsubscribe first, since
// the bus and systemd both drop those for us as soon as our private connection
// goes away. Once this returns, Events is closed.
func (obj *UnitWatcher) Close() error {
	if obj.conn == nil {
		return nil
	}
	// Everything the goroutine waits on ends when its ctx does, so this
	// can't hang. We wait for it before closing the connection, so that it
	// stops because of the cancel, and not because the bus went away.
	obj.cancel()
	<-obj.done
	err := obj.conn.Close()
	obj.conn = nil
	return err
}

// UnitPath returns the object path that systemd uses for the named unit, eg:
// sshd.service becomes /org/freedesktop/systemd1/unit/sshd_2eservice. This is
// the escaping that systemd does in bus_label_escape: ASCII letters are kept,
// digits are kept unless they come first, and every other byte becomes an
// underscore followed by two lowercase hex digits. The unit does not have to be
// loaded, or even exist, for this path to be useful in a match rule.
func UnitPath(unit string) dbus.ObjectPath {
	if unit == "" {
		return dbus.ObjectPath(unitPathPrefix + "_") // systemd special case
	}
	var b strings.Builder
	b.WriteString(unitPathPrefix)
	for i := 0; i < len(unit); i++ {
		c := unit[i]
		alpha := ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
		digit := '0' <= c && c <= '9'
		if alpha || (digit && i > 0) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "_%02x", c)
	}
	return dbus.ObjectPath(b.String())
}

// UnitNameMangle turns a name into one that's allowed in a unit name, the same
// way that systemctl does with the names you give it, eg: foo! becomes foo\x21.
// This is the escaping that systemd does in unit_name_mangle: ASCII letters,
// digits and the characters :-_.\@ are kept, a slash becomes a dash, and every
// other byte becomes \x followed by two lowercase hex digits. Unlike with
// systemd-escape, a name that's already allowed doesn't change, so a dash, an
// instance such as getty@tty1, or something that's already escaped, are all
// kept as they are. Do this first, and then UnitPath on the result.
func UnitNameMangle(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		alpha := ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
		digit := '0' <= c && c <= '9'
		if alpha || digit || strings.IndexByte(`:-_.\@`, c) >= 0 {
			b.WriteByte(c)
			continue
		}
		if c == '/' {
			b.WriteByte('-')
			continue
		}
		fmt.Fprintf(&b, `\x%02x`, c)
	}
	return b.String()
}

// UnitFileState returns the enablement state of the named unit, eg: enabled or
// disabled. It asks the manager, which reads the unit files from disk each
// time, the same as systemctl is-enabled does. Don't read the UnitFileState
// property of the unit instead, since systemd caches that until the next
// daemon-reload, so it misses any enable or disable that happened since, even
// one done over the bus.
func UnitFileState(ctx context.Context, conn *dbus.Conn, unit string) (string, error) {
	obj := conn.Object(BusName, ManagerPath)
	var state string
	call := obj.CallWithContext(ctx, ManagerInterface+".GetUnitFileState", 0, unit)
	if err := call.Store(&state); err != nil {
		return "", errwrap.Wrapf(err, "failed to get unit file state")
	}
	return state, nil
}

// subscribe asks systemd to send out its signals. It only sends most of them if
// at least one client on the bus has asked for them, and it forgets about us
// when we disconnect, so we never need to call Unsubscribe.
func subscribe(ctx context.Context, conn *dbus.Conn) error {
	obj := conn.Object(BusName, ManagerPath)
	err := obj.CallWithContext(ctx, ManagerInterface+".Subscribe", 0).Err
	var e dbus.Error
	if errors.As(err, &e) && e.Name == alreadySubscribed {
		return nil // that's what we wanted anyway
	}
	if err != nil {
		return errwrap.Wrapf(err, "failed to subscribe to systemd")
	}
	return nil
}

// activeState pulls the ActiveState and SubState out of the body of a unit's
// PropertiesChanged signal, eg: "active (running)", or returns the empty string
// if they're not in there.
func activeState(body []interface{}) string {
	if len(body) < 2 {
		return ""
	}
	changed, ok := body[1].(map[string]dbus.Variant)
	if !ok {
		return ""
	}
	active, _ := changed["ActiveState"].Value().(string)
	if active == "" {
		return ""
	}
	if sub, _ := changed["SubState"].Value().(string); sub != "" {
		return fmt.Sprintf("%s (%s)", active, sub)
	}
	return active
}

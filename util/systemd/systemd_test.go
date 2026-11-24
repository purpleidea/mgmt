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

package systemd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func TestUnitPath(t *testing.T) {
	tests := map[string]string{
		"":                     "_",
		"sshd.service":         "sshd_2eservice",
		"avahi-daemon.service": "avahi_2ddaemon_2eservice",
		"getty@tty1.service":   "getty_40tty1_2eservice",
		"foo_bar.service":      "foo_5fbar_2eservice",
		"1foo.service":         "_31foo_2eservice",
		"foo1.service":         "foo1_2eservice",
		"a\\x2db.mount":        "a_5cx2db_2emount",
		"\x01":                 "_01", // two hex digits, always
	}
	for unit, label := range tests {
		expected := dbus.ObjectPath(unitPathPrefix + label)
		path := UnitPath(unit)
		if path != expected {
			t.Errorf("unit %q: got %q, expected %q", unit, path, expected)
		}
		if !path.IsValid() {
			t.Errorf("unit %q: path %q is not valid", unit, path)
		}
	}
}

func TestUnitNameMangle(t *testing.T) {
	tests := map[string]string{
		"":             "",
		"sshd":         "sshd",
		"avahi-daemon": "avahi-daemon", // systemd-escape would change this
		"getty@tty1":   "getty@tty1",
		"a:b_c.d":      "a:b_c.d",
		`a\x2db`:       `a\x2db`, // already escaped
		"foo!":         `foo\x21`,
		"foo bar":      `foo\x20bar`,
		"a/b":          "a-b",
		"foo*":         `foo\x2a`, // not a glob, unlike systemctl
		"\x00":         `\x00`,
		"\x01":         `\x01`,     // two hex digits, always
		"é":            `\xc3\xa9`, // each byte on its own
	}
	for name, expected := range tests {
		if s := UnitNameMangle(name); s != expected {
			t.Errorf("name %q: got %q, expected %q", name, s, expected)
		}
		if s := UnitNameMangle(expected); s != expected {
			t.Errorf("name %q: mangled again: %q", expected, s)
		}
	}
}

func TestActiveState(t *testing.T) {
	tests := []struct {
		body     []interface{}
		expected string
	}{
		{nil, ""},
		{[]interface{}{UnitInterface}, ""},
		{[]interface{}{UnitInterface, "bad"}, ""},
		{[]interface{}{UnitInterface, map[string]dbus.Variant{}}, ""},
		{
			[]interface{}{UnitInterface, map[string]dbus.Variant{
				"SubState": dbus.MakeVariant("running"),
			}},
			"",
		},
		{
			[]interface{}{UnitInterface, map[string]dbus.Variant{
				"ActiveState": dbus.MakeVariant("inactive"),
			}},
			"inactive",
		},
		{
			[]interface{}{UnitInterface, map[string]dbus.Variant{
				"ActiveState": dbus.MakeVariant("active"),
				"SubState":    dbus.MakeVariant("running"),
			}},
			"active (running)",
		},
	}
	for i, tc := range tests {
		if s := activeState(tc.body); s != tc.expected {
			t.Errorf("test %d: got %q, expected %q", i, s, tc.expected)
		}
	}
}

// propertiesChanged returns a fake PropertiesChanged signal for a unit.
func propertiesChanged(path dbus.ObjectPath, active string) *dbus.Signal {
	return &dbus.Signal{
		Path: path,
		Name: propertiesInterface + ".PropertiesChanged",
		Body: []interface{}{
			UnitInterface,
			map[string]dbus.Variant{"ActiveState": dbus.MakeVariant(active)},
			[]string{},
		},
	}
}

// fakeWatcher starts a watcher goroutine which reads from an unbuffered channel
// instead of the bus. It returns a function which sends one signal, and returns
// once the goroutine has received it. Since the goroutine only receives again
// once it's done with the previous signal, a send also proves that everything
// sent before it has been completely processed.
func fakeWatcher(ctx context.Context, t *testing.T) (*UnitWatcher, chan *dbus.Signal, func(*dbus.Signal)) {
	obj := &UnitWatcher{
		Unit: "foo.service",
		path: UnitPath("foo.service"),
	}
	ch := make(chan *dbus.Signal) // unbuffered, see above
	obj.start(ctx, ch)

	send := func(sig *dbus.Signal) {
		select {
		case ch <- sig:
		case <-time.After(5 * time.Second):
			t.Fatalf("goroutine did not receive the signal")
		}
	}
	return obj, ch, send
}

// waitClosed waits for the Events channel to close, and returns Err.
func waitClosed(t *testing.T, obj *UnitWatcher) error {
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-obj.Events():
			if !ok {
				return obj.Err()
			}
		case <-timeout:
			t.Fatalf("events channel did not close")
		}
	}
}

func TestUnitWatcherEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obj, ch, send := fakeWatcher(ctx, t)

	// barrier makes sure that everything sent so far was processed.
	barrier := func() {
		send(&dbus.Signal{Name: dbusInterface + ".NameAcquired"})
	}

	// pending returns the waiting event, if there is one.
	pending := func() (string, bool) {
		select {
		case msg, ok := <-obj.Events():
			if !ok {
				t.Fatalf("events closed early: %v", obj.Err())
			}
			return msg, true
		default:
			return "", false
		}
	}

	// These must all be ignored.
	send(propertiesChanged(UnitPath("bar.service"), "active"))
	send(&dbus.Signal{
		Path: ManagerPath,
		Name: ManagerInterface + ".Reloading",
		Body: []interface{}{true}, // a reload is starting
	})
	barrier()
	if msg, ok := pending(); ok {
		t.Fatalf("unexpected event: %s", msg)
	}

	// A burst collapses into the first event.
	send(propertiesChanged(obj.path, "activating"))
	send(propertiesChanged(obj.path, "active"))
	send(&dbus.Signal{
		Path: ManagerPath,
		Name: ManagerInterface + ".Reloading",
		Body: []interface{}{false}, // a reload is done
	})
	barrier()
	if msg, ok := pending(); !ok || msg != "unit is activating" {
		t.Fatalf("got: %q (%t), expected the first event", msg, ok)
	}
	if msg, ok := pending(); ok {
		t.Fatalf("burst was not collapsed, also got: %s", msg)
	}

	// Once that's been received, the next change is a new event.
	send(&dbus.Signal{
		Path: ManagerPath,
		Name: ManagerInterface + ".UnitFilesChanged",
	})
	barrier()
	if msg, ok := pending(); !ok || msg != "unit files changed" {
		t.Fatalf("got: %q (%t), expected unit files changed", msg, ok)
	}

	// Losing the connection stops it, and says why.
	close(ch)
	if err := waitClosed(t, obj); err == nil || err.Error() != "bus connection closed" {
		t.Fatalf("got: %v, expected bus connection closed", err)
	}
}

func TestUnitWatcherCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	obj, _, send := fakeWatcher(ctx, t)

	// Leave an event waiting, it must not stop the goroutine from exiting.
	send(propertiesChanged(obj.path, "active"))

	cancel()
	if err := waitClosed(t, obj); !errors.Is(err, context.Canceled) {
		t.Fatalf("got: %v, expected context canceled", err)
	}
}

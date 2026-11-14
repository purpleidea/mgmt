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

//go:build !root

package packagekit

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"slices"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestNewPkError(t *testing.T) {
	err := newPkError([]interface{}{
		PkErrorEnumPackageAlreadyInstalled,
		"higher version is already installed",
	})
	pkErr, ok := err.(*PkError)
	if !ok {
		t.Fatalf("expected PkError, got: %T", err)
	}
	if pkErr.Code != PkErrorEnumPackageAlreadyInstalled {
		t.Errorf("unexpected code: got %d, want %d", pkErr.Code, PkErrorEnumPackageAlreadyInstalled)
	}
	if pkErr.Details != "higher version is already installed" {
		t.Errorf("unexpected details: %s", pkErr.Details)
	}
}

func TestNewPkErrorInvalidBody(t *testing.T) {
	if err := newPkError([]interface{}{"bad", "body"}); err == nil {
		t.Errorf("expected error")
	}
}

func TestPkInfoEnum(t *testing.T) {
	// these are plain values on the wire, not a bitfield like the filters
	if PkInfoEnumUnknown != 0 || PkInfoEnumInstalled != 1 || PkInfoEnumAvailable != 2 || PkInfoEnumUnavailable != 25 || PkInfoEnumLast != 31 {
		t.Errorf("incorrect packagekit info enum values")
	}
}

func TestIsInstalledList(t *testing.T) {
	conn := newTestConn(t, [][]interface{}{
		{uint32(1), "automatic;1;all;auto:debian", ""},
		{uint32(1), "manual;1;all;manual:debian", ""},
		{uint32(1), "empty;1;all;", ""},
		{uint32(1), "fedora;1;noarch;installed:fedora", ""},
		{uint32(2), "available;1;all;auto:debian", ""},
		{uint32(2), "misleading;1;all;installed", ""},
	}, nil)
	got, err := conn.IsInstalledList(context.Background(), []string{"automatic", "manual", "empty", "fedora", "available", "misleading", "missing", "automatic"})
	if err != nil {
		t.Fatalf("func IsInstalledList: %v", err)
	}
	if want := []bool{true, true, true, true, false, false, false, true}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPackagesToPackageIDs(t *testing.T) {
	const installedID = "pkg;1;all;auto:debian"
	const availableID = "pkg;2;all;manual:debian"
	installed := []interface{}{uint32(1), installedID, ""}
	available := []interface{}{uint32(2), availableID, ""}
	testCases := []struct {
		name     string
		state    string
		resolved [][]interface{}
		updates  []string
		want     PkPackageIDActionData
	}{
		{"installed", "installed", [][]interface{}{installed}, nil, PkPackageIDActionData{true, true, "1", installedID, true}},
		{"installed with update", "installed", [][]interface{}{installed, available}, []string{availableID}, PkPackageIDActionData{true, true, "1", installedID, false}},
		{"remove installed first", "uninstalled", [][]interface{}{installed, available}, []string{availableID}, PkPackageIDActionData{true, true, "1", installedID, false}},
		{"remove installed last", "uninstalled", [][]interface{}{available, installed}, []string{availableID}, PkPackageIDActionData{true, true, "1", installedID, false}},
		{"update installed first", "newest", [][]interface{}{installed, available}, []string{availableID}, PkPackageIDActionData{true, true, "1", availableID, false}},
		{"update installed last", "newest", [][]interface{}{available, installed}, []string{availableID}, PkPackageIDActionData{true, true, "1", availableID, false}},
		{"pin available", "2", [][]interface{}{installed, available}, []string{availableID}, PkPackageIDActionData{true, true, "1", availableID, false}},
		{"pin installed", "1", [][]interface{}{installed, available}, []string{availableID}, PkPackageIDActionData{true, true, "1", installedID, false}},
		{"pin installed before same version", "1", [][]interface{}{installed, {uint32(2), "pkg;1;all;manual:debian", ""}}, nil, PkPackageIDActionData{true, true, "1", installedID, true}},
		{"pin installed after same version", "1", [][]interface{}{{uint32(2), "pkg;1;all;manual:debian", ""}, installed}, nil, PkPackageIDActionData{true, true, "1", installedID, true}},
		{"install missing", "installed", [][]interface{}{available}, nil, PkPackageIDActionData{true, false, "", availableID, false}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn := newTestConn(t, tc.resolved, tc.updates)
			got, err := conn.PackagesToPackageIDs(context.Background(), map[string]string{"pkg": tc.state}, 0)
			if err != nil {
				t.Fatalf("func PackagesToPackageIDs: %v", err)
			}
			if got["pkg"] == nil || *got["pkg"] != tc.want {
				t.Errorf("got %+v, want %+v", got["pkg"], tc.want)
			}
		})
	}
}

func TestResolvePackagesInfo(t *testing.T) {
	const id = "pkg;1;all;debian"
	conn := newTestConn(t, [][]interface{}{
		{uint32(2), id, ""},
		{uint32(1), id, ""},
		{uint32(1), id, ""}, // duplicate
	}, nil)
	resolved, err := conn.ResolvePackages(context.Background(), []string{"pkg"}, 0)
	if err != nil {
		t.Fatalf("func ResolvePackages: %v", err)
	}
	if len(resolved) != 2 || resolved[0].Info != PkInfoEnumAvailable || resolved[1].Info != PkInfoEnumInstalled {
		t.Fatalf("lost package state: %+v", resolved)
	}
	installed, err := conn.IsInstalled(context.Background(), "pkg")
	if err != nil {
		t.Fatalf("func IsInstalled: %v", err)
	}
	if !installed {
		t.Errorf("expected pkg to be installed")
	}
}

func TestResolvePackagesInvalidBody(t *testing.T) {
	for _, body := range [][]interface{}{
		{},
		{uint32(1)},
		{uint64(1), "pkg;1;all;auto", ""},
		{uint32(1), uint32(2), ""},
		{uint32(1), "pkg;1;all", ""},
	} {
		conn := newTestConn(t, [][]interface{}{body}, nil)
		if _, err := conn.ResolvePackages(context.Background(), []string{"pkg"}, 0); err == nil {
			t.Errorf("expected error for body: %v", body)
		}
	}
}

// newTestConn serves just the bus and PackageKit methods needed by these tests
// over an in-memory connection. No system daemon or package changes are needed.
func newTestConn(t *testing.T, resolved [][]interface{}, updates []string) *Conn {
	t.Helper()
	client, server := net.Pipe()
	conn, err := dbus.NewConn(client)
	if err != nil {
		_ = client.Close()
		_ = server.Close()
		t.Fatalf("func NewConn: %v", err)
	}
	// The test owns this server goroutine. Closing both pipe ends unblocks
	// reads and writes; cleanup joins it before the test returns.
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		done <- serveTestConn(server, resolved, updates)
	}()
	t.Cleanup(func() {
		_ = conn.Close()
		_ = server.Close()
		if err := <-done; err != nil && err != io.EOF && err != io.ErrClosedPipe {
			t.Errorf("test bus: %v", err)
		}
	})
	if err := conn.Auth(nil); err != nil {
		t.Fatalf("func Auth: %v", err)
	}
	return &Conn{conn: conn, Logf: t.Logf}
}

// serveTestConn is the fake bus and PackageKit daemon run by newTestConn. Each
// Resolve call emits the resolved packages, and each GetUpdates call emits the
// updates, followed by a Finished signal.
func serveTestConn(server net.Conn, resolved [][]interface{}, updates []string) error {
	reader := bufio.NewReader(server)
	for _, step := range []struct{ request, response string }{
		{"\x00AUTH\r\n", "REJECTED EXTERNAL\r\n"},
		{"AUTH EXTERNAL\r\n", "OK 0123456789abcdef0123456789abcdef\r\n"},
		{"BEGIN\r\n", ""},
	} {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		if line != step.request {
			return fmt.Errorf("unexpected auth request: %q", line)
		}
		if step.response != "" {
			if _, err := io.WriteString(server, step.response); err != nil {
				return err
			}
		}
	}
	const path = dbus.ObjectPath("/org/freedesktop/PackageKit/transactions/test")
	send := func(msg *dbus.Message) error {
		if len(msg.Body) > 0 {
			msg.Headers[dbus.FieldSignature] = dbus.MakeVariant(dbus.SignatureOf(msg.Body...))
		}
		return msg.EncodeTo(server, binary.LittleEndian)
	}
	signal := func(member string, body []interface{}) error {
		return send(&dbus.Message{
			Type: dbus.TypeSignal,
			Headers: map[dbus.HeaderField]dbus.Variant{
				dbus.FieldPath:      dbus.MakeVariant(path),
				dbus.FieldInterface: dbus.MakeVariant(PkIfaceTransaction),
				dbus.FieldMember:    dbus.MakeVariant(member),
			},
			Body: body,
		})
	}
	for {
		msg, err := dbus.DecodeMessage(reader)
		if err != nil {
			return err
		}
		member, ok := msg.Headers[dbus.FieldMember].Value().(string)
		if !ok {
			return fmt.Errorf("message has no member: %v", msg)
		}
		body := []interface{}{}
		switch member {
		case "CreateTransaction":
			body = append(body, path)
		case "AddMatch", "RemoveMatch", "Resolve", "GetUpdates":
		default:
			return fmt.Errorf("unexpected method: %s", member)
		}
		if err := send(&dbus.Message{
			Type: dbus.TypeMethodReply,
			Headers: map[dbus.HeaderField]dbus.Variant{
				dbus.FieldReplySerial: dbus.MakeVariant(msg.Serial()),
			},
			Body: body,
		}); err != nil {
			return err
		}
		if member != "Resolve" && member != "GetUpdates" {
			continue
		}
		packages := resolved
		if member == "GetUpdates" {
			packages = [][]interface{}{}
			for _, id := range updates {
				packages = append(packages, []interface{}{uint32(5), id, ""}) // normal
			}
		}
		for _, body := range packages {
			if err := signal("Package", body); err != nil {
				return err
			}
		}
		if err := signal("Finished", []interface{}{uint32(1), uint32(0)}); err != nil {
			return err
		}
	}
}

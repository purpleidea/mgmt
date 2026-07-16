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
	"io"
	"math"
	"os"
	"testing"

	"github.com/purpleidea/mgmt/engine"

	"github.com/godbus/dbus/v5"
)

func TestFwupdDictHelpers(t *testing.T) {
	m := map[string]dbus.Variant{
		"Bool":    dbus.MakeVariant(true),
		"Str":     dbus.MakeVariant("hello"),
		"Strs":    dbus.MakeVariant([]string{"a", "b"}),
		"Uint32":  dbus.MakeVariant(uint32(42)),
		"Uint64":  dbus.MakeVariant(uint64(42)),
		"WrongTy": dbus.MakeVariant(3.14),
	}

	if !fwupdDictBool(m, "Bool") {
		t.Errorf("func fwupdDictBool: got false")
	}
	if fwupdDictBool(m, "Missing") {
		t.Errorf("func fwupdDictBool on missing key: got true")
	}
	if x := fwupdDictStr(m, "Str"); x != "hello" {
		t.Errorf("func fwupdDictStr: got %q", x)
	}
	if x := fwupdDictStr(m, "Missing"); x != "" {
		t.Errorf("func fwupdDictStr on missing key: got %q", x)
	}
	if x := fwupdDictStr(m, "WrongTy"); x != "" {
		t.Errorf("func fwupdDictStr on wrong type: got %q", x)
	}
	if x := fwupdDictStrs(m, "Strs"); len(x) != 2 || x[0] != "a" || x[1] != "b" {
		t.Errorf("func fwupdDictStrs: got %v", x)
	}
	if x := fwupdDictStrs(m, "Missing"); len(x) != 0 {
		t.Errorf("func fwupdDictStrs on missing key: got %v", x)
	}
	if x := fwupdDictUint64(m, "Uint32"); x != 42 {
		t.Errorf("func fwupdDictUint64 on u: got %d", x)
	}
	if x := fwupdDictUint64(m, "Uint64"); x != 42 {
		t.Errorf("func fwupdDictUint64 on t: got %d", x)
	}
	if x := fwupdDictUint64(m, "Missing"); x != 0 {
		t.Errorf("func fwupdDictUint64 on missing key: got %d", x)
	}
}

func TestFwupdDeviceMatches(t *testing.T) {
	device := newFwupdDevice(map[string]dbus.Variant{
		"DeviceId":    dbus.MakeVariant("08d460be0f1f9f128413f816022a6439e0078018"),
		"Name":        dbus.MakeVariant("Fake webcam"),
		"Guid":        dbus.MakeVariant([]string{"b585990a-003e-5270-89d5-3705a17f9a43"}),
		"Version":     dbus.MakeVariant("1.2.2"),
		"Flags":       dbus.MakeVariant(fwupdDeviceFlagUpdatable),
		"UpdateState": dbus.MakeVariant(uint32(0)),
	})

	if !device.Matches("b585990a-003e-5270-89d5-3705a17f9a43") {
		t.Errorf("func Matches: guid did not match")
	}
	if !device.Matches("B585990A-003E-5270-89D5-3705A17F9A43") {
		t.Errorf("func Matches: guid was case sensitive")
	}
	if !device.Matches("08d460be0f1f9f128413f816022a6439e0078018") {
		t.Errorf("func Matches: device-id did not match")
	}
	if device.Matches("00000000-0000-0000-0000-000000000000") {
		t.Errorf("func Matches: wrong guid matched")
	}
	if !device.IsUpdatable() {
		t.Errorf("func IsUpdatable: got false")
	}

	device = newFwupdDevice(map[string]dbus.Variant{
		"UpdateState": dbus.MakeVariant(uint64(math.MaxUint32) + 1 + uint64(fwupdUpdateStateNeedsReboot)),
	})
	if device.UpdateState != 0 {
		t.Errorf("func newFwupdDevice on overflowing update state: got %d", device.UpdateState)
	}
}

func TestFwupdNewRelease(t *testing.T) {
	// a modern daemon sends Locations and Flags...
	release := newFwupdRelease(map[string]dbus.Variant{
		"Version":   dbus.MakeVariant("1.2.4"),
		"Locations": dbus.MakeVariant([]string{"https://example.com/x.cab"}),
		"Uri":       dbus.MakeVariant("https://example.com/old-uri.cab"),
		"Checksum":  dbus.MakeVariant([]string{"abc"}),
		"Flags":     dbus.MakeVariant(fwupdReleaseFlagIsUpgrade),
	})
	uri, err := release.Location()
	if err != nil {
		t.Fatalf("func Location: %v", err)
	}
	if uri != "https://example.com/x.cab" {
		t.Errorf("func Location: got %q", uri)
	}
	if !release.IsUpgrade() || release.IsDowngrade() {
		t.Errorf("func IsUpgrade/IsDowngrade: got %v/%v", release.IsUpgrade(), release.IsDowngrade())
	}

	// ...while an older daemon sends Uri and TrustFlags instead
	release = newFwupdRelease(map[string]dbus.Variant{
		"Version":    dbus.MakeVariant("1.2.1"),
		"Uri":        dbus.MakeVariant("https://example.com/y.cab"),
		"TrustFlags": dbus.MakeVariant(fwupdReleaseFlagIsDowngrade),
	})
	uri, err = release.Location()
	if err != nil {
		t.Fatalf("func Location: %v", err)
	}
	if uri != "https://example.com/y.cab" {
		t.Errorf("func Location on uri fallback: got %q", uri)
	}
	if !release.IsDowngrade() {
		t.Errorf("func IsDowngrade on flags fallback: got false")
	}

	release = newFwupdRelease(map[string]dbus.Variant{
		"Version": dbus.MakeVariant("1.2.0"),
	})
	if _, err := release.Location(); err == nil {
		t.Errorf("func Location without any: expected error")
	}
}

func TestFwupdRemoteResolveURI(t *testing.T) {
	// a mirror remote rewrites everything except the basename
	remote := &fwupdRemote{
		RemoteID:        "mirror",
		MetadataURI:     "http://mirror.lan/downloads/firmware.xml.zst",
		FirmwareBaseURI: "http://mirror.lan/downloads",
	}
	if x := remote.ResolveURI("https://fwupd.org/downloads/x.cab"); x != "http://mirror.lan/downloads/x.cab" {
		t.Errorf("func ResolveURI with base uri: got %q", x)
	}
	remote.FirmwareBaseURI = "http://mirror.lan/downloads/" // trailing slash
	if x := remote.ResolveURI("https://fwupd.org/downloads/x.cab"); x != "http://mirror.lan/downloads/x.cab" {
		t.Errorf("func ResolveURI with trailing slash base uri: got %q", x)
	}

	// without a base uri, a relative location is joined to the metadata dir
	remote = &fwupdRemote{
		RemoteID:    "lvfs",
		MetadataURI: "https://cdn.fwupd.org/downloads/firmware.xml.zst",
	}
	if x := remote.ResolveURI("x.cab"); x != "https://cdn.fwupd.org/downloads/x.cab" {
		t.Errorf("func ResolveURI on relative location: got %q", x)
	}

	// absolute locations pass through untouched
	if x := remote.ResolveURI("https://fwupd.org/downloads/x.cab"); x != "https://fwupd.org/downloads/x.cab" {
		t.Errorf("func ResolveURI on absolute uri: got %q", x)
	}
	if x := remote.ResolveURI("/usr/share/fwupd/remotes.d/fwupd-tests/x.cab"); x != "/usr/share/fwupd/remotes.d/fwupd-tests/x.cab" {
		t.Errorf("func ResolveURI on absolute path: got %q", x)
	}

	remote = newFwupdRemote(map[string]dbus.Variant{
		"ModificationTime": dbus.MakeVariant(uint64(math.MaxInt64) + 1),
	})
	if remote.ModificationTime != 0 {
		t.Errorf("func newFwupdRemote on overflowing modification time: got %d", remote.ModificationTime)
	}
}

func TestFwupdVerifyChecksums(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "fwupd-test-")
	if err != nil {
		t.Fatalf("func CreateTemp: %v", err)
	}
	defer f.Close()
	if _, err := f.WriteString("hello world\n"); err != nil {
		t.Fatalf("func WriteString: %v", err)
	}
	// $ printf 'hello world\n' | sha1sum ; printf 'hello world\n' | sha256sum
	sha1sum := "22596363b3de40b06f981fb85d82312e8c0ed511"
	sha256sum := "a948904f2f0f479b8f8197694b30184b0d2ed1c1cd2a1ec0fb85d299a192a447"

	if err := fwupdVerifyChecksums(f, []string{sha256sum}); err != nil {
		t.Errorf("func fwupdVerifyChecksums on sha256: %v", err)
	}
	if err := fwupdVerifyChecksums(f, []string{sha1sum}); err != nil {
		t.Errorf("func fwupdVerifyChecksums on sha1: %v", err)
	}
	if err := fwupdVerifyChecksums(f, []string{"deadbeef" + sha1sum[8:], sha1sum}); err != nil {
		t.Errorf("func fwupdVerifyChecksums on sha1 fallback: %v", err)
	}
	if err := fwupdVerifyChecksums(f, []string{sha1sum, "deadbeef" + sha256sum[8:]}); err == nil {
		t.Errorf("func fwupdVerifyChecksums on stronger mismatch: expected error")
	}
	if err := fwupdVerifyChecksums(f, []string{}); err != nil {
		t.Errorf("func fwupdVerifyChecksums on empty list: %v", err)
	}
	if err := fwupdVerifyChecksums(f, []string{"deadbeef" + sha256sum[8:]}); err == nil {
		t.Errorf("func fwupdVerifyChecksums on mismatch: expected error")
	}

	// the file must be seeked back to the start after a successful verify
	if err := fwupdVerifyChecksums(f, []string{sha256sum}); err != nil {
		t.Fatalf("func fwupdVerifyChecksums: %v", err)
	}
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("func ReadAll: %v", err)
	}
	if string(b) != "hello world\n" {
		t.Errorf("func fwupdVerifyChecksums did not rewind, read: %q", string(b))
	}
}

func TestFwupdRemoteUIDIFF(t *testing.T) {
	lvfs := &FwupdRemoteUID{remote: "lvfs"}
	tests := &FwupdRemoteUID{remote: "fwupd-tests"}
	wildcard := &FwupdRemoteUID{remote: ""}

	if !lvfs.IFF(lvfs) {
		t.Errorf("func IFF: same remote did not match")
	}
	if lvfs.IFF(tests) {
		t.Errorf("func IFF: different remotes matched")
	}
	if !wildcard.IFF(lvfs) || !wildcard.IFF(tests) {
		t.Errorf("func IFF: the wildcard did not match")
	}
	if lvfs.IFF(&FwupdDeviceUID{device: "lvfs"}) {
		t.Errorf("func IFF: another uid type matched")
	}
}

func TestFwupdAutoEdges(t *testing.T) {
	// every wanted uid must come out exactly once, matched or not
	uids := []engine.ResUID{
		&FwupdRemoteUID{remote: "lvfs"},
		&FwupdRemoteUID{remote: ""},
	}
	autoEdge := &FwupdAutoEdges{uids: uids}

	x := autoEdge.Next()
	if len(x) != 1 || x[0] != uids[0] {
		t.Fatalf("func Next: got %v", x)
	}
	if !autoEdge.Test([]bool{true}) { // a match must not stop the sequence
		t.Fatalf("func Test: stopped early")
	}
	x = autoEdge.Next()
	if len(x) != 1 || x[0] != uids[1] {
		t.Fatalf("func Next: got %v", x)
	}
	if autoEdge.Test([]bool{false}) {
		t.Fatalf("func Test: did not stop at the end")
	}
}

func TestFwupdSignalMatchesDevice(t *testing.T) {
	guid := "b585990a-003e-5270-89d5-3705a17f9a43"
	body := map[string]dbus.Variant{
		"DeviceId": dbus.MakeVariant("08d460be0f1f9f128413f816022a6439e0078018"),
		"Guid":     dbus.MakeVariant([]string{guid}),
	}

	signal := &dbus.Signal{
		Name: fwupdDBusInterface + ".Changed",
	}
	if !fwupdSignalMatchesDevice(signal, guid) {
		t.Errorf("func fwupdSignalMatchesDevice: the Changed signal must always match")
	}

	signal = &dbus.Signal{
		Name: fwupdDBusInterface + ".DeviceChanged",
		Body: []interface{}{body},
	}
	if !fwupdSignalMatchesDevice(signal, guid) {
		t.Errorf("func fwupdSignalMatchesDevice: our device did not match")
	}
	if fwupdSignalMatchesDevice(signal, "00000000-0000-0000-0000-000000000000") {
		t.Errorf("func fwupdSignalMatchesDevice: another device matched")
	}

	signal = &dbus.Signal{
		Name: fwupdDBusInterface + ".DeviceChanged",
		Body: []interface{}{"garbage"},
	}
	if !fwupdSignalMatchesDevice(signal, guid) {
		t.Errorf("func fwupdSignalMatchesDevice: an undecodable signal must match")
	}
}

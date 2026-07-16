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
	"crypto/sha1" //nolint:gosec // G505: fwupd supports legacy SHA-1 archive checksums
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/godbus/dbus/v5"
)

const (
	// fwupdDBusName is the well-known bus name of the fwupd daemon.
	fwupdDBusName = "org.freedesktop.fwupd"

	// fwupdDBusPath is the singleton object path of the fwupd daemon.
	fwupdDBusPath = dbus.ObjectPath("/")

	// fwupdDBusInterface is the interface all the fwupd methods and signals
	// are found on.
	fwupdDBusInterface = "org.freedesktop.fwupd"

	// fwupdDeviceFlagUpdatable is the FwupdDeviceFlags bit which tells us
	// the device can receive firmware updates.
	fwupdDeviceFlagUpdatable = uint64(1) << 1

	// fwupdReleaseFlagIsUpgrade is the FwupdReleaseFlags bit which tells us
	// a release is newer than the version currently on the device. These
	// comparisons are done by the daemon which knows the version format of
	// each device (quad, semver, intel-me, and so on) so we never have to
	// compare version strings ourselves.
	fwupdReleaseFlagIsUpgrade = uint64(1) << 2

	// fwupdReleaseFlagIsDowngrade is the FwupdReleaseFlags bit which tells
	// us a release is older than the version currently on the device.
	fwupdReleaseFlagIsDowngrade = uint64(1) << 3

	// fwupdUpdateStateNeedsReboot is the FwupdUpdateState value which tells
	// us an update was deployed and is waiting for a reboot to apply.
	fwupdUpdateStateNeedsReboot = uint32(4)

	// fwupdMetadataSignatureSuffix is appended to a remote's metadata uri
	// to find the detached jcat signature that the daemon verifies.
	fwupdMetadataSignatureSuffix = ".jcat"
)

// fwupdClient is a private connection to the fwupd daemon on the system bus.
// The fwupdmgr cli is only a thin client of this same D-Bus API, so anything it
// can do, we can do here in pure golang, without wrapping any binaries.
type fwupdClient struct {
	conn   *dbus.Conn
	object dbus.BusObject
}

// newFwupdClient connects to the fwupd daemon. This will dbus-activate the
// daemon if it's not running yet. Don't forget to call Close when done.
func newFwupdClient() (*fwupdClient, error) {
	conn, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return nil, errwrap.Wrapf(err, "failed to connect to the private system bus")
	}
	return &fwupdClient{
		conn:   conn,
		object: conn.Object(fwupdDBusName, fwupdDBusPath),
	}, nil
}

// Close disconnects from the bus.
func (obj *fwupdClient) Close() error {
	return obj.conn.Close()
}

// Devices returns all the devices the daemon knows about.
func (obj *fwupdClient) Devices(ctx context.Context) ([]*fwupdDevice, error) {
	raw := []map[string]dbus.Variant{}
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".GetDevices", 0)
	if err := call.Store(&raw); err != nil {
		return nil, errwrap.Wrapf(err, "the GetDevices method failed")
	}
	devices := []*fwupdDevice{}
	for _, m := range raw {
		devices = append(devices, newFwupdDevice(m))
	}
	return devices, nil
}

// Device finds a single device by GUID or by daemon device-id.
func (obj *fwupdClient) Device(ctx context.Context, id string) (*fwupdDevice, error) {
	devices, err := obj.Devices(ctx)
	if err != nil {
		return nil, err
	}
	for _, device := range devices {
		if device.Matches(id) {
			return device, nil
		}
	}
	return nil, fmt.Errorf("device %s was not found", id)
}

// Releases returns all the releases the metadata knows about for one device.
// The daemon returns these ordered from newest to oldest. It errors if there is
// no metadata at all for the device.
func (obj *fwupdClient) Releases(ctx context.Context, deviceID string) ([]*fwupdRelease, error) {
	raw := []map[string]dbus.Variant{}
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".GetReleases", 0, deviceID)
	if err := call.Store(&raw); err != nil {
		return nil, errwrap.Wrapf(err, "the GetReleases method failed")
	}
	releases := []*fwupdRelease{}
	for _, m := range raw {
		releases = append(releases, newFwupdRelease(m))
	}
	return releases, nil
}

// Remotes returns all the remotes the daemon knows about.
func (obj *fwupdClient) Remotes(ctx context.Context) ([]*fwupdRemote, error) {
	raw := []map[string]dbus.Variant{}
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".GetRemotes", 0)
	if err := call.Store(&raw); err != nil {
		return nil, errwrap.Wrapf(err, "the GetRemotes method failed")
	}
	remotes := []*fwupdRemote{}
	for _, m := range raw {
		remotes = append(remotes, newFwupdRemote(m))
	}
	return remotes, nil
}

// ResolveURI rewrites a release download location according to the rules of the
// remote it came from, if any. An empty or unknown remote id leaves the uri
// unchanged.
func (obj *fwupdClient) ResolveURI(ctx context.Context, remoteID, uri string) (string, error) {
	if remoteID == "" {
		return uri, nil
	}
	remotes, err := obj.Remotes(ctx)
	if err != nil {
		return "", err
	}
	for _, remote := range remotes {
		if remote.RemoteID == remoteID {
			return remote.ResolveURI(uri), nil
		}
	}
	return uri, nil
}

// ModifyRemote changes a single key of the daemon's remote configuration.
func (obj *fwupdClient) ModifyRemote(ctx context.Context, remoteID, key, value string) error {
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".ModifyRemote", 0, remoteID, key, value)
	return errwrap.Wrapf(call.Err, "the ModifyRemote method failed")
}

// UpdateMetadata hands freshly downloaded metadata and its detached signature
// to the daemon, which verifies and imports it. This is how a "refresh" works:
// the client (us) does the network transfer, and the daemon does the rest.
func (obj *fwupdClient) UpdateMetadata(ctx context.Context, remoteID string, data, signature *os.File) error {
	//nolint:gosec // G115: Unix file descriptors are kernel int values; godbus requires int32
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".UpdateMetadata", 0, remoteID, dbus.UnixFD(data.Fd()), dbus.UnixFD(signature.Fd()))
	return errwrap.Wrapf(call.Err, "the UpdateMetadata method failed")
}

// Install asks the daemon to install the firmware archive that our open file
// descriptor points at, onto the given device. The daemon verifies the archive
// signature itself. Flashing can take minutes, so make sure the input ctx has
// an appropriate lifetime. The option keys are the legacy string booleans (eg:
// "allow-older") which both the 1.x and 2.x daemons accept.
func (obj *fwupdClient) Install(ctx context.Context, deviceID string, f *os.File, options map[string]bool) error {
	opts := map[string]dbus.Variant{}
	for key, value := range options {
		if !value {
			continue // only send the keys which are set
		}
		opts[key] = dbus.MakeVariant(true)
	}
	//nolint:gosec // G115: Unix file descriptors are kernel int values; godbus requires int32
	call := obj.object.CallWithContext(ctx, fwupdDBusInterface+".Install", 0, deviceID, dbus.UnixFD(f.Fd()), opts)
	return errwrap.Wrapf(call.Err, "the Install method failed")
}

// fwupdDevice is the decoded form of the a{sv} device dict that the daemon
// returns from GetDevices and sends with its Device* signals. Only the fields
// that we use are included.
type fwupdDevice struct {
	// DeviceID is the unique (40 hex char) id the daemon gave this device.
	DeviceID string

	// Name is the human readable device name.
	Name string

	// Guids are the stable GUIDs that identify this device model.
	Guids []string

	// Version is the firmware version currently on the device.
	Version string

	// Flags is the FwupdDeviceFlags bit field.
	Flags uint64

	// UpdateState is the FwupdUpdateState of the last update, if any.
	UpdateState uint32
}

// Matches returns true if the given identifier refers to this device. It can be
// either one of the device GUIDs or the daemon device-id.
func (obj *fwupdDevice) Matches(id string) bool {
	if strings.EqualFold(id, obj.DeviceID) {
		return true
	}
	for _, guid := range obj.Guids {
		if strings.EqualFold(id, guid) {
			return true
		}
	}
	return false
}

// IsUpdatable returns true if the daemon says this device can be updated.
func (obj *fwupdDevice) IsUpdatable() bool {
	return obj.Flags&fwupdDeviceFlagUpdatable != 0
}

// newFwupdDevice decodes a device from the daemon's a{sv} representation.
func newFwupdDevice(m map[string]dbus.Variant) *fwupdDevice {
	return &fwupdDevice{
		DeviceID:    fwupdDictStr(m, "DeviceId"),
		Name:        fwupdDictStr(m, "Name"),
		Guids:       fwupdDictStrs(m, "Guid"),
		Version:     fwupdDictStr(m, "Version"),
		Flags:       fwupdDictUint64(m, "Flags"),
		UpdateState: fwupdDictUint32(m, "UpdateState"),
	}
}

// fwupdRelease is the decoded form of the a{sv} release dict that the daemon
// returns from GetReleases. Only the fields that we use are included.
type fwupdRelease struct {
	// Version is the firmware version this release contains.
	Version string

	// RemoteID is the remote this release was found in.
	RemoteID string

	// Locations are the uris the firmware archive can be fetched from.
	Locations []string

	// Checksums are the known checksums (sha1 or sha256 hex strings) of
	// the firmware archive. These come from the gpg/jcat signed metadata.
	Checksums []string

	// Flags is the FwupdReleaseFlags bit field.
	Flags uint64
}

// IsUpgrade returns true if the daemon says this release is newer than what is
// currently on the device.
func (obj *fwupdRelease) IsUpgrade() bool {
	return obj.Flags&fwupdReleaseFlagIsUpgrade != 0
}

// IsDowngrade returns true if the daemon says this release is older than what
// is currently on the device.
func (obj *fwupdRelease) IsDowngrade() bool {
	return obj.Flags&fwupdReleaseFlagIsDowngrade != 0
}

// Location returns the first place we can download this release from.
func (obj *fwupdRelease) Location() (string, error) {
	if len(obj.Locations) == 0 {
		return "", fmt.Errorf("release %s has no download location", obj.Version)
	}
	return obj.Locations[0], nil
}

// newFwupdRelease decodes a release from the daemon's a{sv} representation.
func newFwupdRelease(m map[string]dbus.Variant) *fwupdRelease {
	locations := fwupdDictStrs(m, "Locations") // newer daemons
	if uri := fwupdDictStr(m, "Uri"); uri != "" && len(locations) == 0 {
		locations = []string{uri} // older daemons
	}
	flags := fwupdDictUint64(m, "Flags")
	if flags == 0 { // very old daemons used a different key
		flags = fwupdDictUint64(m, "TrustFlags")
	}
	return &fwupdRelease{
		Version:   fwupdDictStr(m, "Version"),
		RemoteID:  fwupdDictStr(m, "RemoteId"),
		Locations: locations,
		Checksums: fwupdDictStrs(m, "Checksum"),
		Flags:     flags,
	}
}

// fwupdRemote is the decoded form of the a{sv} remote dict that the daemon
// returns from GetRemotes. Only the fields that we use are included.
type fwupdRemote struct {
	// RemoteID is the unique id of this remote, eg: "lvfs".
	RemoteID string

	// Enabled is true if the daemon will use this remote.
	Enabled bool

	// MetadataURI is where the metadata for this remote is downloaded from.
	MetadataURI string

	// FirmwareBaseURI is an optional replacement prefix for the firmware
	// download locations found in the metadata. This is how mirrors work:
	// the metadata still names the upstream lvfs location, and this
	// redirects the download to the mirror instead.
	FirmwareBaseURI string

	// ModificationTime is when the local metadata copy was last updated,
	// in seconds since the epoch. Zero if it was never downloaded.
	ModificationTime int64
}

// ResolveURI applies this remote's rewriting rules to a release download
// location, matching the libfwupd client behaviour: a remote with a
// FirmwareBaseUri (eg: a local mirror) replaces everything except the basename,
// and a location without any scheme is taken as relative to where the metadata
// itself came from.
func (obj *fwupdRemote) ResolveURI(uri string) string {
	if obj.FirmwareBaseURI != "" {
		return strings.TrimSuffix(obj.FirmwareBaseURI, "/") + "/" + path.Base(uri)
	}
	hasScheme := strings.Contains(uri, "://")
	if !hasScheme && !strings.HasPrefix(uri, "/") && obj.MetadataURI != "" {
		if i := strings.LastIndex(obj.MetadataURI, "/"); i >= 0 {
			return obj.MetadataURI[:i+1] + uri
		}
	}
	return uri
}

// newFwupdRemote decodes a remote from the daemon's a{sv} representation.
func newFwupdRemote(m map[string]dbus.Variant) *fwupdRemote {
	return &fwupdRemote{
		RemoteID:         fwupdDictStr(m, "RemoteId"),
		Enabled:          fwupdDictBool(m, "Enabled"),
		MetadataURI:      fwupdDictStr(m, "Uri"),
		FirmwareBaseURI:  fwupdDictStr(m, "FirmwareBaseUri"),
		ModificationTime: fwupdDictInt64(m, "ModificationTime"),
	}
}

// fwupdWatch is the shared Watch implementation for the fwupd resources. It
// connects to the system bus and subscribes to all the signals on the daemon
// interface: DeviceAdded, DeviceRemoved, DeviceChanged, DeviceRequest, and the
// coarse Changed signal which the daemon emits when anything else important
// happens, notably when remote metadata is refreshed or reconfigured. This is
// what gives these resources real events without any polling. The optional
// filter can drop signals which aren't relevant to us, and the optional tick
// channel produces extra events, for resources with a time based component.
func fwupdWatch(ctx context.Context, init *engine.Init, tick <-chan time.Time, filter func(*dbus.Signal) bool) error {
	bus, err := util.SystemBusPrivateUsable() // don't share the bus connection!
	if err != nil {
		return errwrap.Wrapf(err, "failed to connect to bus")
	}
	defer bus.Close()

	args := fmt.Sprintf("type='signal', interface='%s'", fwupdDBusInterface)
	if call := bus.BusObject().Call(engineUtil.DBusAddMatch, 0, args); call.Err != nil {
		return errwrap.Wrapf(call.Err, "failed to subscribe to the fwupd signals")
	}
	defer bus.BusObject().Call(engineUtil.DBusRemoveMatch, 0, args) // ignore the error

	signals := make(chan *dbus.Signal, 10) // closed by dbus package
	bus.Signal(signals)

	if err := init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case signal, ok := <-signals:
			if !ok { // channel shutdown
				return fmt.Errorf("unexpected close")
			}
			if init.Debug {
				init.Logf("event: %s", signal.Name)
			}
			if filter != nil && !filter(signal) {
				continue
			}

		case <-tick: // nil chan if unused, which blocks forever
			// pass

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		if err := init.Event(ctx); err != nil {
			return err
		}
	}
}

// FwupdAutoEdges is a simple auto edge generator which yields each of its
// wanted uids exactly once, whether they match or not.
type FwupdAutoEdges struct {
	uids    []engine.ResUID
	pointer int
}

// Next returns the next automatic edge.
func (obj *FwupdAutoEdges) Next() []engine.ResUID {
	if obj.pointer >= len(obj.uids) {
		return nil
	}
	value := obj.uids[obj.pointer]
	obj.pointer++
	return []engine.ResUID{value} // we return one, even though api supports N
}

// Test takes the output of the last call to Next() and outputs true if we
// should continue.
func (obj *FwupdAutoEdges) Test(input []bool) bool {
	return obj.pointer < len(obj.uids) // are there any more left?
}

// fwupdConsumerAutoEdges creates the edges which put every fwupd:remote
// resource before a resource that consumes firmware releases. We ask the daemon
// which remote ids actually exist, so that each matching fwupd:remote resource
// in the graph gets a precise edge, and we always append one wildcard as well,
// which catches a remote that this graph is in the middle of creating (eg: a
// conf file the daemon hasn't loaded yet) and costs at most one extra, still
// valid, edge. If the daemon isn't reachable, the wildcard alone gives us at
// least one edge.
func fwupdConsumerAutoEdges(ctx context.Context, name, kind string) (engine.AutoEdge, error) {
	reversed := true // fwupd:remote happens before the consumer
	base := engine.BaseUID{Name: name, Kind: kind, Reversed: &reversed}
	uids := []engine.ResUID{}
	if client, err := newFwupdClient(); err == nil {
		defer client.Close()
		if remotes, err := client.Remotes(ctx); err == nil {
			for _, remote := range remotes {
				uids = append(uids, &FwupdRemoteUID{
					BaseUID: base,
					remote:  remote.RemoteID,
				})
			}
		}
	}
	uids = append(uids, &FwupdRemoteUID{
		BaseUID: base,
		remote:  "", // wildcard, matches any fwupd:remote resource
	})
	return &FwupdAutoEdges{uids: uids}, nil
}

// fwupdInstall downloads a release, verifies its checksum, and asks the daemon
// to flash it onto the given device. The download location is first resolved
// through the rules of the remote the release came from, notably the
// FirmwareBaseUri mirror redirection. The daemon additionally verifies the
// archive signature itself before flashing anything.
func fwupdInstall(ctx context.Context, client *fwupdClient, device *fwupdDevice, release *fwupdRelease, options map[string]bool) error {
	uri, err := release.Location()
	if err != nil {
		return err
	}
	uri, err = client.ResolveURI(ctx, release.RemoteID, uri)
	if err != nil {
		return err
	}
	f, err := fwupdDownload(ctx, uri)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := fwupdVerifyChecksums(f, release.Checksums); err != nil {
		return err
	}

	return client.Install(ctx, device.DeviceID, f, options)
}

// fwupdDownload fetches a firmware archive or metadata file and returns it as
// an open, unlinked temporary file, seeked to the start, which is suitable for
// fd-passing to the daemon. It supports http(s) uris for normal remotes like
// the lvfs, and file uris or plain paths for local (directory kind) remotes
// like the built-in fwupd-tests one.
func fwupdDownload(ctx context.Context, uri string) (*os.File, error) {
	if strings.HasPrefix(uri, "file://") {
		return os.Open(strings.TrimPrefix(uri, "file://"))
	}
	if strings.HasPrefix(uri, "/") {
		return os.Open(uri)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, errwrap.Wrapf(err, "invalid uri: %s", uri)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errwrap.Wrapf(err, "download failed: %s", uri)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: %s (%s)", uri, resp.Status)
	}

	f, err := os.CreateTemp("", "mgmt-fwupd-")
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not create a temporary file")
	}
	// unlink it right away, the open fd keeps it alive until we close it
	if err := os.Remove(f.Name()); err != nil {
		f.Close() //nolint:gosec // G104: preserve the primary unlink error
		return nil, errwrap.Wrapf(err, "could not unlink the temporary file")
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close() //nolint:gosec // G104: preserve the primary download error
		return nil, errwrap.Wrapf(err, "download failed: %s", uri)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close() //nolint:gosec // G104: preserve the primary seek error
		return nil, errwrap.Wrapf(err, "could not seek the temporary file")
	}
	return f, nil
}

// fwupdVerifyChecksums checks the file contents against a list of known hex
// checksums, as found in the signed metadata. It uses the strongest supported
// kind, sha256 ahead of sha1, and passes if any checksum of that kind matches.
// On success the file is seeked back to the start for fd-passing. An empty
// checksum list passes, since local test remotes don't provide any.
func fwupdVerifyChecksums(f *os.File, checksums []string) error {
	if len(checksums) == 0 {
		return nil // nothing known to verify against
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errwrap.Wrapf(err, "could not seek the file")
	}
	bestLength := 0
	for _, checksum := range checksums {
		switch len(checksum) {
		case 2 * sha256.Size:
			bestLength = 2 * sha256.Size
		case 2 * sha1.Size:
			if bestLength == 0 {
				bestLength = 2 * sha1.Size
			}
		}
	}

	var hasher hash.Hash
	switch bestLength {
	case 2 * sha256.Size:
		hasher = sha256.New()
	case 2 * sha1.Size:
		hasher = sha1.New() //nolint:gosec // G401: fwupd supports legacy SHA-1 archive checksums
	default:
		return fmt.Errorf("checksum mismatch, expected one of: %s", strings.Join(checksums, ", "))
	}
	if _, err := io.Copy(hasher, f); err != nil {
		return errwrap.Wrapf(err, "could not hash the file")
	}
	computed := hex.EncodeToString(hasher.Sum(nil))

	for _, checksum := range checksums {
		if len(checksum) == bestLength && strings.EqualFold(checksum, computed) {
			_, err := f.Seek(0, io.SeekStart)
			return errwrap.Wrapf(err, "could not seek the file")
		}
	}

	return fmt.Errorf("checksum mismatch, expected one of: %s", strings.Join(checksums, ", "))
}

// fwupdSignalMatchesDevice decides if a daemon signal is relevant to the given
// device identifier. The Device* signals carry the device dict as their body,
// so those can be filtered precisely. The bare Changed signal carries nothing,
// so it always matches, since we can't know who it was about.
func fwupdSignalMatchesDevice(signal *dbus.Signal, id string) bool {
	if signal.Name == fwupdDBusInterface+".Changed" {
		return true
	}
	if len(signal.Body) != 1 {
		return true // unexpected shape, assume it's relevant
	}
	m, ok := signal.Body[0].(map[string]dbus.Variant)
	if !ok {
		return true // unexpected shape, assume it's relevant
	}
	return newFwupdDevice(m).Matches(id)
}

// fwupdDictBool looks up a boolean out of an a{sv} dict.
func fwupdDictBool(m map[string]dbus.Variant, key string) bool {
	variant, exists := m[key]
	if !exists {
		return false
	}
	b, ok := variant.Value().(bool)
	if !ok {
		return false
	}
	return b
}

// fwupdDictStr looks up a single string out of an a{sv} dict.
func fwupdDictStr(m map[string]dbus.Variant, key string) string {
	variant, exists := m[key]
	if !exists {
		return ""
	}
	s, ok := variant.Value().(string)
	if !ok {
		return ""
	}
	return s
}

// fwupdDictStrs looks up a list of strings out of an a{sv} dict.
func fwupdDictStrs(m map[string]dbus.Variant, key string) []string {
	variant, exists := m[key]
	if !exists {
		return []string{}
	}
	xs, ok := variant.Value().([]string)
	if !ok {
		return []string{}
	}
	return xs
}

// fwupdDictUint32 looks up an unsigned 32 bit integer out of an a{sv} dict.
func fwupdDictUint32(m map[string]dbus.Variant, key string) uint32 {
	x := fwupdDictUint64(m, key)
	if x > math.MaxUint32 {
		return 0
	}
	return uint32(x)
}

// fwupdDictInt64 looks up a non-negative signed integer out of an a{sv} dict.
func fwupdDictInt64(m map[string]dbus.Variant, key string) int64 {
	x := fwupdDictUint64(m, key)
	if x > math.MaxInt64 {
		return 0
	}
	return int64(x)
}

// fwupdDictUint64 looks up an unsigned integer out of an a{sv} dict. The daemon
// uses both the u (uint32) and t (uint64) types for these.
func fwupdDictUint64(m map[string]dbus.Variant, key string) uint64 {
	variant, exists := m[key]
	if !exists {
		return 0
	}
	switch x := variant.Value().(type) {
	case uint64:
		return x
	case uint32:
		return uint64(x)
	}
	return 0
}

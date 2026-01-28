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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"

	"github.com/bmc-toolbox/common"
	fwUtil "github.com/metal-automata/fw"
)

func init() {
	engine.RegisterResource("bmc:firmware", func() engine.Res { return &BmcFirmwareRes{} })
}

const (
	// DefaultBmcFirmwarePort is the default port we try to connect on.
	DefaultBmcFirmwarePort = 443
)

// BmcFirmwareRes is a resource that updates the firmware on a BMC. This is
// usually done for server grade hardware that requires special API's to perform
// these actions. This interacts on similar channels as the bmc:power resource
// and this resource can block the execution of the former as that may be
// required during the update process.
//
// The use of this resource may cause your server to restart, be prepared for
// this scenario before attempting an update.
type BmcFirmwareRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Hostname to connect to. If not specified, we parse this from the
	// Name.
	Hostname string `lang:"hostname" yaml:"hostname"`

	// Port to connect to. If not specified, we parse this from the Name.
	Port int `lang:"port" yaml:"port"`

	// Username to use to connect. If not specified, we parse this from the
	// Name.
	// TODO: If the Username field is not set, should we parse from the
	// Name? It's not really part of the BMC unique identifier so maybe we
	// shouldn't use that.
	Username string `lang:"username" yaml:"username"`

	// Password to use to connect. We do NOT parse this from the Name unless
	// you set InsecurePassword to true.
	// XXX: Use mgmt magic credentials in the future.
	Password string `lang:"password" yaml:"password"`

	// InsecurePassword can be set to true to allow a password in the Name.
	InsecurePassword bool `lang:"insecure_password" yaml:"insecure_password"`

	// Provider is the machine provider to use. Eg: "supermicro".
	Provider string `lang:"provider" yaml:"provider"`

	// File path to the firmware binary to install.
	File string `lang:"file" yaml:"file"`

	// Hash is a sha256 sum of the File. This is used for integrity checking
	// before doing an upload. It's optional to use this field, but it is
	// recommended.
	Hash string `lang:"hash" yaml:"hash"`

	// Version is the expected resultant version string of this firmware
	// binary. This needs to be specified since we can't always inspect the
	// file to know what version it contains.
	Version string `lang:"version" yaml:"version"`

	// Block is the Name of a bmc:power resource that should be blocked when
	// running a firmware update.
	Block string `lang:"block" yaml:"block"`
}

// validDriver determines if we are using a valid drive. This does not include
// the magic "s" bits. This function need to be expanded as we support more
// drivers.
func (obj *BmcFirmwareRes) validDriver(driver string) error {
	if driver == BmcDriverRPC {
		return nil
	}
	if driver == BmcDriverGofish {
		return nil
	}

	return fmt.Errorf("unknown driver: %s", driver)
}

// getHostname returns the hostname that we want to connect to. If the Hostname
// field is set, we use that, otherwise we parse from the Name.
func (obj *BmcFirmwareRes) getHostname() string {
	if obj.Hostname != "" {
		return obj.Hostname
	}

	u, err := url.Parse(obj.Name())
	if err != nil || u == nil {
		return ""
	}

	// SplitHostPort splits a network address of the form "host:port",
	// "host%zone:port", "[host]:port" or "[host%zone]:port" into host or
	// host%zone and port.
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return u.Host // must be a naked hostname or ip w/o port
	}
	_ = port

	return host
}

// getPort returns the port that we want to connect to. If the Port field is
// set, we use that, otherwise we parse from the Name.
//
// NOTE: We return a string since all the bmclib things usually expect a string,
// but if that gets fixed we should return an int here instead.
func (obj *BmcFirmwareRes) getPort() string {
	if obj.Port != 0 {
		return strconv.Itoa(obj.Port)
	}

	u, err := url.Parse(obj.Name())
	if err != nil || u == nil {
		return ""
	}

	// SplitHostPort splits a network address of the form "host:port",
	// "host%zone:port", "[host]:port" or "[host%zone]:port" into host or
	// host%zone and port.
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		return strconv.Itoa(DefaultBmcFirmwarePort) // default port
	}
	_ = host

	return port
}

// getUsername returns the username that we want to connect with. If the
// Username field is set, we use that, otherwise we parse from the Name.
// TODO: If the Username field is not set, should we parse from the Name? It's
// not really part of the BMC unique identifier so maybe we shouldn't use that.
func (obj *BmcFirmwareRes) getUsername() string {
	if obj.Username != "" {
		return obj.Username
	}

	u, err := url.Parse(obj.Name())
	if err != nil || u == nil || u.User == nil {
		return ""
	}

	return u.User.Username()
}

// getPassword returns the password that we want to connect with.
// XXX: Use mgmt magic credentials in the future.
func (obj *BmcFirmwareRes) getPassword() string {
	if obj.Password != "" || !obj.InsecurePassword {
		return obj.Password
	}
	// NOTE: We don't look at any password string from the name unless the
	// InsecurePassword field is true.

	u, err := url.Parse(obj.Name())
	if err != nil || u == nil || u.User == nil {
		return ""
	}

	password, ok := u.User.Password()
	if !ok {
		return ""
	}

	return password
}

// Default returns some sensible defaults for this resource.
func (obj *BmcFirmwareRes) Default() engine.Res {
	return &BmcFirmwareRes{}
}

// Validate if the params passed in are valid data.
func (obj *BmcFirmwareRes) Validate() error {
	// XXX: Force polling until we have real events...
	if obj.MetaParams().Poll == 0 {
		return fmt.Errorf("events are not yet supported, use polling")
	}

	if obj.getHostname() == "" {
		return fmt.Errorf("need a Hostname")
	}
	//if obj.getUsername() == "" {
	//	return fmt.Errorf("need a Username")
	//}

	if obj.Provider == "" {
		return fmt.Errorf("the Provider is empty")
	}

	if !strings.HasPrefix(obj.File, "/") {
		return fmt.Errorf("the File must be an absolute path")
	}
	if strings.HasSuffix(obj.File, "/") {
		return fmt.Errorf("the File must not be a directory")
	}

	// TODO: can we support a "yolo" variant without a version?
	if obj.Version == "" {
		return fmt.Errorf("the Version is empty")
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *BmcFirmwareRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *BmcFirmwareRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *BmcFirmwareRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply method for BmcFirmware resource. Does nothing, returns happy!
func (obj *BmcFirmwareRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	bs := bmcStateReserve(obj.Block) // get a handle to the shared bmc state
	defer bs.Release()

	// We only want one resource touching the bmc at a time.
	// XXX: put the lock lower perhaps?
	bs.Lock()
	defer bs.Unlock()

	component := common.SlugBMC // "BMC"

	installer := &fwUtil.Installer{
		//DryRun: true,
		BMCAddr:      obj.getHostname(), // TODO: port?
		Component:    component,
		Username:     obj.getUsername(),
		Password:     obj.getPassword(),
		Vendor:       obj.Provider, // eg: "supermicro"
		Version:      obj.Version,
		FirmwareFile: obj.File,

		Debug: obj.init.Debug, // TODO: add true?
		Logf: func(format string, v ...interface{}) {
			obj.init.Logf("bmc: "+format, v...)
		},
	}
	if err := installer.Connect(ctx); err != nil {
		return false, nil
	}
	// Don't pass in regular ctx because we want to make sure this frees...
	// TODO: add a timeout to the close ctx?
	defer installer.Close(context.Background())

	version, err := installer.GetVersion(ctx)
	if err != nil {
		return false, err
	}
	obj.init.Logf("got version: %s", version)

	if version == obj.Version {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	// XXX: There is a TOC TOU issue here in that someone could edit the
	// file. To do better, we need to read this in to memory, and after
	// hashing it, send it to the upload via an io.Reader. Unfortunately the
	// firmware upload API doesn't support that interface yet. This was
	// because (1) it needed to seek (io.ReadSeeker would be fine) and (2)
	// some variants required a filename too. (We could pass that in too!)
	if obj.Hash != "" {
		h, err := obj.hashFile(obj.File)
		if err != nil {
			return false, err
		}
		if h != obj.Hash {
			return false, fmt.Errorf("the Hash does not match, got: %s", h)
		}
	}

	// We might wish to block the power state changes and reconciliation by
	// other resources while this operation is underway. That's what the top
	// Lock/Unlock is for.
	obj.init.Logf("installing bmc firmware...")
	if err := installer.Install(ctx); err != nil {
		return false, err
	}
	//actions, err := client.FirmwareInstallSteps(ctx, component) // (actions []constants.FirmwareInstallStep, err error)

	obj.init.Logf("installed bmc version: %s", obj.Version)

	return false, nil
}

// hashContent is a simple helper to run our hashing function.
func (obj *BmcFirmwareRes) hashContent(handle io.Reader) (string, error) {
	hash := sha256.New()
	if _, err := io.Copy(hash, handle); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashFile is a helper that returns the hash of the specified file. If the file
// doesn't exist, it returns the empty string. Otherwise it errors.
func (obj *BmcFirmwareRes) hashFile(file string) (string, error) {
	f, err := os.Open(file) // io.Reader
	if err != nil && !os.IsNotExist(err) {
		// This is likely a permissions error.
		return "", err

	} else if err != nil {
		return "", err // File doesn't exist!
	}

	defer f.Close()

	// File exists, lets hash it!

	return obj.hashContent(f)
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *BmcFirmwareRes) Cmp(r engine.Res) error {
	// we can only compare BmcFirmwareRes to others of the same resource kind
	res, ok := r.(*BmcFirmwareRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.Hostname != res.Hostname {
		return fmt.Errorf("the Hostname differs")
	}
	if obj.Port != res.Port {
		return fmt.Errorf("the Port differs")
	}
	if obj.Username != res.Username {
		return fmt.Errorf("the Username differs")
	}
	if obj.Password != res.Password {
		return fmt.Errorf("the Password differs")
	}
	if obj.InsecurePassword != res.InsecurePassword {
		return fmt.Errorf("the InsecurePassword differs")
	}

	if obj.Provider != res.Provider {
		return fmt.Errorf("the Provider differs")
	}
	if obj.File != res.File {
		return fmt.Errorf("the File differs")
	}
	if obj.File != res.File {
		return fmt.Errorf("the File differs")
	}
	if obj.Hash != res.Hash {
		return fmt.Errorf("the Hash differs")
	}
	if obj.Version != res.Version {
		return fmt.Errorf("the Version differs")
	}

	if obj.Block != res.Block {
		return fmt.Errorf("the Block differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *BmcFirmwareRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes BmcFirmwareRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*BmcFirmwareRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to BmcFirmwareRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = BmcFirmwareRes(raw) // restore from indirection with type conversion!
	return nil
}

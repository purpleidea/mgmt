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
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"

	bmclib "github.com/bmc-toolbox/bmclib/v2"
	"github.com/bmc-toolbox/bmclib/v2/providers/rpc"
)

func init() {
	engine.RegisterResource("bmc:power", func() engine.Res { return &BmcPowerRes{} })
}

const (
	// DefaultBmcPowerPort is the default port we try to connect on.
	DefaultBmcPowerPort = 443

	// BmcDriverSecureSuffix is the magic char we append to a driver name to
	// specify we want the SSL/TLS variant.
	BmcDriverSecureSuffix = "s"

	// BmcDriverRPC is the RPC driver.
	BmcDriverRPC = "rpc"

	// BmcDriverGofish is the gofish driver.
	BmcDriverGofish = "gofish"
)

// BmcPowerRes is a resource that manages power state of a BMC. This is usually
// used for turning computers on and off. The name value can be a big URL string
// in the form: `driver://user:pass@hostname:port` for example you may see:
// gofishs://ADMIN:hunter2@127.0.0.1:8800 to use the "https" variant of the
// gofish driver.
//
// NOTE: New drivers should either not end in "s" or at least not be identical
// to the name of another driver an "s" is added or removed to the end.
type BmcPowerRes struct {
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

	// Driver to use, such as: "gofish" or "rpc". This is a different
	// concept than the "bmclib" driver vs provider distinction. Here we
	// just statically pick what we're using without any magic. If not
	// specified, we parse this from the Name scheme. If this ends with an
	// extra "s" then we use https instead of http.
	Driver string `lang:"driver" yaml:"driver"`

	// State of machine power. Can be "on" or "off".
	State string `lang:"state" yaml:"state"`

	driver string
	scheme string
}

// validDriver determines if we are using a valid drive. This does not include
// the magic "s" bits. This function need to be expanded as we support more
// drivers.
func (obj *BmcPowerRes) validDriver(driver string) error {
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
func (obj *BmcPowerRes) getHostname() string {
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
func (obj *BmcPowerRes) getPort() string {
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
		return strconv.Itoa(DefaultBmcPowerPort) // default port
	}
	_ = host

	return port
}

// getUsername returns the username that we want to connect with. If the
// Username field is set, we use that, otherwise we parse from the Name.
// TODO: If the Username field is not set, should we parse from the Name? It's
// not really part of the BMC unique identifier so maybe we shouldn't use that.
func (obj *BmcPowerRes) getUsername() string {
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
func (obj *BmcPowerRes) getPassword() string {
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

// getRawDriver returns the raw magic driver string. If the Driver field is set,
// we use that, otherwise we parse from the Name. This version may include the
// magic "s" at the end.
func (obj *BmcPowerRes) getRawDriver() string {
	if obj.Driver != "" {
		return obj.Driver
	}

	u, err := url.Parse(obj.Name())
	if err != nil || u == nil {
		return ""
	}

	return u.Scheme
}

// getDriverAndScheme figures out which driver and scheme we want to use.
func (obj *BmcPowerRes) getDriverAndScheme() (string, string, error) {
	driver := obj.getRawDriver()
	err := obj.validDriver(driver)
	if err == nil {
		return driver, "http", nil
	}

	driver = strings.TrimSuffix(driver, BmcDriverSecureSuffix)
	if err := obj.validDriver(driver); err == nil {
		return driver, "https", nil
	}

	return "", "", err // return the first error
}

// getDriver returns the actual driver that we want to connect with. If the
// Driver field is set, we use that, otherwise we parse from the Name. This
// version does NOT include the magic "s" at the end.
func (obj *BmcPowerRes) getDriver() string {
	return obj.driver
}

// getScheme figures out which scheme we want to use.
func (obj *BmcPowerRes) getScheme() string {
	return obj.scheme
}

// Default returns some sensible defaults for this resource.
func (obj *BmcPowerRes) Default() engine.Res {
	return &BmcPowerRes{}
}

// Validate if the params passed in are valid data.
func (obj *BmcPowerRes) Validate() error {
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

	if obj.getRawDriver() == "" {
		return fmt.Errorf("need a Driver")
	}
	if _, _, err := obj.getDriverAndScheme(); err != nil {
		return err
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *BmcPowerRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	driver, scheme, err := obj.getDriverAndScheme()
	if err != nil {
		// programming error (we checked in Validate)
		return err
	}
	obj.driver = driver
	obj.scheme = scheme

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *BmcPowerRes) Cleanup() error {
	return nil
}

// client builds the bmclib client. The API to build it is complicated.
func (obj *BmcPowerRes) client() *bmclib.Client {
	// NOTE: The bmclib API is weird, you can't put the port in this string!
	u := fmt.Sprintf("%s://%s", obj.getScheme(), obj.getHostname())

	uPort := u
	if p := obj.getPort(); p != "" {
		uPort = u + ":" + p
	}

	opts := []bmclib.Option{}

	if obj.getDriver() == BmcDriverRPC {
		opts = append(opts, bmclib.WithRPCOpt(rpc.Provider{
			// NOTE: The main API cannot take a port, but here we do!
			ConsumerURL: uPort,
		}))
	}

	if p := obj.getPort(); p != "" {
		switch obj.getDriver() {
		case BmcDriverRPC:
			// TODO: ???

		case BmcDriverGofish:
			// XXX: Why doesn't this accept an int?
			opts = append(opts, bmclib.WithRedfishPort(p))

		//case BmcDriverOpenbmc:
		//	// XXX: Why doesn't this accept an int?
		//	opts = append(opts, openbmc.WithPort(p))

		default:
			// TODO: error or pass through?
			obj.init.Logf("unhandled driver: %s", obj.getDriver())
		}
	}

	client := bmclib.NewClient(u, obj.getUsername(), obj.Password, opts...)

	if obj.getDriver() != "" && obj.getDriver() != BmcDriverRPC {
		client = client.For(obj.getDriver()) // limit to this provider
	}

	return client
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *BmcPowerRes) Watch(ctx context.Context) error {
	obj.init.Running() // when started, notify engine that we're running

	select {
	case <-ctx.Done(): // closed by the engine to signal shutdown
	}

	//obj.init.Event() // notify engine of an event (this can block)

	return nil
}

// CheckApply method for BmcPower resource. Does nothing, returns happy!
func (obj *BmcPowerRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	client := obj.client()

	if err := client.Open(ctx); err != nil {
		return false, err
	}
	defer client.Close(ctx) // (err error)

	if obj.init.Debug {
		obj.init.Logf("connected ok")
	}

	state, err := client.GetPowerState(ctx)
	if err != nil {
		return false, err
	}
	state = strings.ToLower(state) // normalize
	obj.init.Logf("get state: %s", state)

	if !apply {
		return false, nil
	}

	if obj.State == state {
		return true, nil
	}

	// TODO: should this be "On" and "Off"? Does case matter?
	ok, err := client.SetPowerState(ctx, obj.State)
	if err != nil {
		return false, err
	}
	if !ok {
		// TODO: When is this ever false?
	}
	obj.init.Logf("set state: %s", obj.State)

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *BmcPowerRes) Cmp(r engine.Res) error {
	// we can only compare BmcPowerRes to others of the same resource kind
	res, ok := r.(*BmcPowerRes)
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

	if obj.Driver != res.Driver {
		return fmt.Errorf("the Driver differs")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *BmcPowerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes BmcPowerRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*BmcPowerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to BmcPowerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = BmcPowerRes(raw) // restore from indirection with type conversion!
	return nil
}

// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

//go:build !nodocker
// +build !nodocker

package resources

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/docker/docker/api/types/network"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("docker:network", func() engine.Res { return &DockerNetworkRes{} })
	engine.RegisterResource("docker:network:address", func() engine.Res { return &DockerNetworkAddressRes{} })
}

// DockerNetworkRes is a docker network resource. The resource's name must be a
// docker network in any supported format (url, network, or network:tag).
type DockerNetworkRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Groupable
	traits.GraphQueryable

	// State of the network must be exists or absent.
	State string `lang:"state"`
	// Force defines whether containers attached to a network should be stopped
	// and removed before removing the network, else CheckApply will fail.
	Force bool `lang:"force"`

	// Driver defines the network driver. Defaults to 'bridge'
	Driver string `lang:"driver"`
	// Enable IPv6 defines whether IPv6 should be enabled in the network
	EnableIPv6 bool `lang:"enableipv6"`
	// Labels are key=value pairs to label the network with
	Labels map[string]string `lang:"labels"`
	// Options are key=value pairs to pass to the network driver
	Options map[string]string `lang:"options"`

	// TODO: Add basic address block to docker:network when lang allows anonymous struct members
	// IPAM defines IP address blocks/gateways for the network.
	// Embed a single IPAM block within the resource. Additional blocks can
	// be provided by autogrouping a docker:network:address Res
	// *IPAM
	// IPAMDriver defines the IPAM driver.
	IPAMDriver string `lang:"ipamdriver"`
	// IPAMOptions are key=value pairs to pass to the IPAM driver
	IPAMOptions map[string]string `lang:"ipamoptions"`

	// APIVersion allows you to override the host's default client API
	// version.
	APIVersion string `lang:"apiversion"`

	client *client.Client // docker api client
	init   *engine.Init
}

// Default returns some sensible defaults for this resource.
func (obj *DockerNetworkRes) Default() engine.Res {
	return &DockerNetworkRes{
		Driver: "bridge",
		Force:  false,
	}
}

// Validate if the params passed in are valid data.
func (obj *DockerNetworkRes) Validate() error {
	// validate state
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be exists or absent")
	}

	/*
		TODO: Add basic address block to docker:network when lang allows anonymous struct members
		if obj.Subnet != "" {
			ipam := DockerNetworkAddressRes{
				Network:    obj.Name(),
				Subnet:     obj.Subnet,
				IPRange:    obj.IPRange,
				Gateway:    obj.Gateway,
				AuxAddress: obj.AuxAddress,
			}
			if err := ipam.Validate(); err != nil {
				return err
			}
		}
	*/
	for name := range obj.IPAMOptions {
		if strings.TrimSpace(name) == "" {
			return errors.New("ipam options must have a name")
		}
	}

	for name := range obj.Labels {
		if strings.TrimSpace(name) == "" {
			return errors.New("labels must have a name")
		}
	}

	for name := range obj.Options {
		if strings.TrimSpace(name) == "" {
			return errors.New("options must have a name")
		}
	}

	if obj.APIVersion != "" {
		verOK, err := regexp.MatchString(`^(v)[1-9]\.[0-9]\d*$`, obj.APIVersion)
		if err != nil {
			return errwrap.Wrapf(err, "error matching apiversion string")
		}
		if !verOK {
			return fmt.Errorf("invalid apiversion: %s", obj.APIVersion)
		}
	}

	return nil
}

// Init runs some startup code for this resource.
func (obj *DockerNetworkRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	// Initialize the docker client.
	var err error
	obj.client, err = client.NewClientWithOpts(client.WithVersion(obj.APIVersion))
	if err != nil {
		return errwrap.Wrapf(err, "error creating docker client")
	}

	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *DockerNetworkRes) Close() error {
	return obj.client.Close() // close the docker client
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DockerNetworkRes) Watch() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan, errChan := obj.client.Events(ctx, types.EventsOptions{})

	// notify engine that we're running
	obj.init.Running()

	var send = false // send event?
	for {
		select {
		case _, ok := <-eventChan:
			if !ok { // channel shutdown
				return nil
			}
			send = true

		case err, ok := <-errChan:
			if !ok {
				return nil
			}
			// TODO: attempt reconnecting briefly in case daemon was restarted
			return err

		case <-obj.init.Done: // closed by the engine to signal shutdown
			return nil
		}

		// do all our event sending all together to avoid duplicate msgs
		if send {
			send = false
			obj.init.Event() // notify engine of an event (this can block)
		}
	}
}

// CheckApply method for Docker resource.
func (obj *DockerNetworkRes) CheckApply(apply bool) (checkOK bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	var id string
	nets, err := obj.client.NetworkList(ctx, types.NetworkListOptions{
		Filters: filters.NewArgs(filters.Arg("name", obj.Name())),
	})
	if err != nil {
		return false, errwrap.Wrapf(err, "error listing networks")
	}

	// Docker NetworkList call filters are partial string matches, so for
	// example "ab" would match networks named "abc" and "abz". Find the network
	// with the exact name match
	for _, n := range nets {
		if n.Name == obj.Name() {
			if id != "" { // duplicate name found
				return false, fmt.Errorf("duplicate networks found")
			}
			id = n.ID // found
		}
	}

	// exit early, we're in a good state
	if obj.State == "absent" && id == "" {
		return true, nil
	}

	var destroy = false
	if obj.State == "exists" && id != "" {
		inspect, err := obj.client.NetworkInspect(ctx, id, types.NetworkInspectOptions{})
		if err != nil {
			return false, errwrap.Wrapf(err, "error inspecting network %s", id)
		}

		// compare obj to inspected Docker network
		err = obj.cmpNetwork(ctx, inspect)
		if err == nil {
			return true, nil
		}

		// needs to be recreated
		destroy = true
		obj.init.Logf("network must be destroyed: %s", err)
	}

	if !apply {
		return false, nil
	}

	if obj.State == "absent" {
		return obj.networkRemove(ctx, id)
	}

	if destroy {
		obj.init.Logf("destroying network")
		_, err := obj.networkRemove(ctx, id)
		if err != nil {
			return false, err
		}
	}

	// TODO: Implement IPAM driver/options configurability
	config := types.NetworkCreate{
		CheckDuplicate: true,
		Driver:         obj.Driver,
		EnableIPv6:     obj.EnableIPv6,
		IPAM: &network.IPAM{
			Driver:  obj.IPAMDriver,
			Options: obj.Options,
			Config:  obj.ipamConfigs(),
		},
		Labels:  obj.Labels,
		Options: obj.Options,
	}

	obj.init.Logf("creating network")
	_, err = obj.client.NetworkCreate(ctx, obj.Name(), config)

	return false, errwrap.Wrapf(err, "error creating network")
}

func (obj *DockerNetworkRes) ipamConfigs() IPAMConfigs {
	var configs []network.IPAMConfig

	for _, res := range obj.GetGroup() {
		ipam, ok := res.(*DockerNetworkAddressRes) // convert from GroupableRes
		if ok {
			configs = append(configs, ipam.IPAMConfig())
		}
	}

	return configs
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DockerNetworkRes) Cmp(r engine.Res) (err error) {
	defer func() {
		obj.init.Logf("Cmp returned %+v", err)
	}()
	// we can only compare DockerNetworkRes to others of the same resource kind
	res, ok := r.(*DockerNetworkRes)
	if !ok {
		return fmt.Errorf("error casting r to *DockerNetworkRes")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Force != res.Force {
		return fmt.Errorf("the Force differs")
	}

	if obj.Driver != res.Driver {
		return fmt.Errorf("the Driver differs")
	}
	if obj.EnableIPv6 != res.EnableIPv6 {
		return fmt.Errorf("the EnableIPv6 differs")
	}
	if !reflect.DeepEqual(obj.Labels, res.Labels) {
		return fmt.Errorf("the Labels differ")
	}
	if !reflect.DeepEqual(obj.Options, res.Options) {
		return fmt.Errorf("the Options differ")
	}

	if obj.IPAMDriver != res.IPAMDriver {
		return fmt.Errorf("the IPAMDriver differs")
	}
	if !reflect.DeepEqual(obj.IPAMOptions, res.IPAMOptions) {
		return fmt.Errorf("the IPAMOptions differ")
	}

	if obj.APIVersion != res.APIVersion {
		return fmt.Errorf("the APIVersion differs")
	}
	return nil
}

func (obj *DockerNetworkRes) networkRemove(ctx context.Context, id string) (bool, error) {
	// Lookup the network with all details, specifically the list of attached
	// containers. Removing a network with containers attached will fail, so we
	// preempt that and remove them from the network.
	inspect, err := obj.client.NetworkInspect(ctx, id, types.NetworkInspectOptions{})
	if err != nil {
		return false, errwrap.Wrapf(err, "error inspecting network %s", id)
	}

	// cannot remove networks with containers attached to them
	if len(inspect.Containers) > 0 {
		// don't accidentally all the containers without permission
		if !obj.Force {
			return false, fmt.Errorf("network has active endpoints. aborting removal")
		}

		// remove all containers, then try again
		for id := range inspect.Containers {
			// TODO: Add an option to disconnect containers from the network
			// instead of removing them.. maybe
			opts := types.ContainerRemoveOptions{
				Force: true,
			}
			obj.init.Logf("removing container %s", inspect.Containers[id].Name)
			if err := obj.client.ContainerRemove(ctx, id, opts); err != nil {
				return false, errwrap.Wrapf(err, "error removing container %s", id)
			}
		}
	}

	obj.init.Logf("removing network")
	if err := obj.client.NetworkRemove(ctx, inspect.ID); err != nil {
		return false, errwrap.Wrapf(err, "error removing network")
	}
	return false, nil
}

func (obj *DockerNetworkRes) cmpNetwork(ctx context.Context, inspect types.NetworkResource) error {
	if obj.Driver != inspect.Driver {
		return fmt.Errorf("the network Driver differs")
	}
	if obj.EnableIPv6 != inspect.EnableIPv6 {
		return fmt.Errorf("the network EnableIPv6 differs")
	}

	// Identify and filter out all address versions not explicitly configured
	// by this resource, and ignore them when comparing against the existing
	// Docker network.
	objCfg := obj.ipamConfigs()
	netCfg := IPAMConfigs(inspect.IPAM.Config)
	versions, err := objCfg.Families()
	if err != nil {
		return errwrap.Wrapf(err, "error parsing network IPAM versions")
	}
	filtered, err := netCfg.FilterVersion(versions)
	if err != nil {
		return errwrap.Wrapf(err, "error filtering network IPAM versions")
	}

	// Compare excluding the IP versions that were added implicitly by Docker.
	// This works in every case except for when the user wants the network to be
	// recreated with a Docker assigned allocation for a version instead of the
	// previously explicitly configured subnet. We can't know which is which, so
	// we just ignore all unconfigured allocations and hope they're still fine.
	if !objCfg.Equal(filtered) {
		return errors.New("the network IPAM Config differs")
	}

	if len(obj.Labels) > 0 && len(inspect.Labels) > 0 &&
		!reflect.DeepEqual(obj.Labels, inspect.Labels) {
		return fmt.Errorf("the network Labels differ")
	}
	if len(obj.Options) > 0 && len(inspect.Options) > 0 &&
		!reflect.DeepEqual(obj.Options, inspect.Options) {
		return fmt.Errorf("the network Options differ")
	}

	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *DockerNetworkRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*DockerNetworkAddressRes)
	if ok {
		// group networks matching this network name
		if res.Network != obj.Name() {
			return fmt.Errorf("resource groups with a different network name")
		}

		return nil
	}

	return fmt.Errorf("resource is not the right kind")
}

// IPVersion represents the version of an Internet Protocol address
type IPVersion int

const (
	IPv4 IPVersion = iota
	IPv6
)

// VersionOfIP returns the IPVersion for a given net.IP address
func VersionOfIP(ip net.IP) IPVersion {
	if ip.To4() != nil {
		return IPv4
	} else {
		return IPv6
	}
}

// DockerNetworkUID is the UID struct for DockerNetworkRes.
type DockerNetworkUID struct {
	engine.BaseUID

	network string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *DockerNetworkRes) UIDs() []engine.ResUID {
	x := &DockerNetworkUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		network: obj.Name(),
	}
	return []engine.ResUID{x}
}

// AutoEdges returns the AutoEdge interface.
func (obj *DockerNetworkRes) AutoEdges() (engine.AutoEdge, error) {
	return nil, nil
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *DockerNetworkUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*DockerNetworkUID)
	if !ok {
		return false
	}
	return obj.network == res.network
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DockerNetworkRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DockerNetworkRes // indirection to avoid infinite recursion

	def := obj.Default()               // get the default
	res, ok := def.(*DockerNetworkRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DockerNetworkRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DockerNetworkRes(raw) // restore from indirection with type conversion!
	return nil
}

type IPAMConfigs []network.IPAMConfig

func (obj IPAMConfigs) Families() ([]IPVersion, error) {
	verCount := make(map[IPVersion]struct{}, 0)

	for _, config := range obj {
		ip, _, err := net.ParseCIDR(config.Subnet)
		if err != nil {
			return nil, err
		}
		verCount[VersionOfIP(ip)] = struct{}{}
	}

	versions := []IPVersion{}
	for ver, _ := range verCount {
		versions = append(versions, ver)
	}
	return versions, nil
}

// FilterVersion filters a slice of IPAMConfig blocks retaining only the IP
// version(s) specified and discarding the rest, returning the filtered slice.
// The original slice is not mutated.
func (obj IPAMConfigs) FilterVersion(versions []IPVersion) (IPAMConfigs, error) {
	filtered := make(IPAMConfigs, 0)

	for _, ipam := range obj {
		ip, _, err := net.ParseCIDR(ipam.Subnet)
		if err != nil {
			return nil, err
		}
		ver := VersionOfIP(ip)

		for _, version := range versions {
			// version should be kept, add it and move on
			if ver == version {
				filtered = append(filtered, ipam)
				break
			}
		}
	}

	return filtered, nil
}

func (obj IPAMConfigs) Equal(other IPAMConfigs) bool {
	if len(obj) != len(other) {
		return false
	}

	for _, cfg := range obj {
		match := false
		for _, otherCfg := range other {
			if cfg.Subnet != otherCfg.Subnet {
				continue
			}
			if cfg.Gateway != otherCfg.Gateway ||
				cfg.IPRange != otherCfg.IPRange ||
				!reflect.DeepEqual(cfg.AuxAddress, otherCfg.AuxAddress) {
				return false
			}
			match = true
		}
		if !match {
			return false
		}
	}
	return true
}

type DockerNetworkAddressRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Groupable

	init *engine.Init

	Network string `lang:"network"`

	Subnet     string            `lang:"subnet"`
	IPRange    string            `lang:"iprange"`
	Gateway    string            `lang:"gateway"`
	AuxAddress map[string]string `lang:"auxaddress"`
}

func (obj *DockerNetworkAddressRes) Validate() error {
	if obj.Network == "" {
		return fmt.Errorf("network must be specified")
	}

	if obj.Subnet == "" {
		return fmt.Errorf("subnet must be specified")
	}

	_, subnet, err := net.ParseCIDR(obj.Subnet)
	if err != nil {
		return errwrap.Wrapf(err, "error parsing subnet")
	}

	if obj.IPRange != "" {
		_, ipr, err := net.ParseCIDR(obj.IPRange)
		if err != nil {
			return errwrap.Wrapf(err, "error parsing iprange")
		}
		if len(ipr.IP) != len(subnet.IP) {
			return errors.New("subnet and iprange must be the same IP version")
		}

		start, end := cidr.AddressRange(subnet)
		if !subnet.Contains(start) || !subnet.Contains(end) {
			return errors.New("iprange must reside within subnet")
		}
	}

	if obj.Gateway != "" {
		gateway := net.ParseIP(obj.Gateway)
		// net.ParseIP returns [16]byte even for IPv4 addresses. To4() returns
		// the IPv4 address in a [4]byte if it's v4, else nil. This gives us
		// [4]byte for IPv4 and [16]byte for IPv6 so we can compare lengths.
		if v4 := gateway.To4(); v4 != nil {
			gateway = v4
		}
		if gateway == nil {
			return errors.New("error parsing gateway")
		}
		if len(gateway) != len(subnet.IP) {
			return errors.New("subnet and gateway must be the same IP version")
		}
		if !subnet.Contains(gateway) {
			return errors.New("gateway must reside within subnet")
		}
	}

	if obj.AuxAddress != nil && len(obj.AuxAddress) > 0 {
		for host, addr := range obj.AuxAddress {
			ip := net.ParseIP(addr)
			if v4 := ip.To4(); v4 != nil {
				ip = v4
			}
			if ip == nil {
				return fmt.Errorf("error parsing address for host %s", host)
			}
			if len(ip) != len(subnet.IP) {
				return fmt.Errorf("subnet and host \"%s\" address must be the same IP version", host)
			}
			if !subnet.Contains(ip) {
				return fmt.Errorf("host \"%s\" address must reside within subnet %s", host, obj.Subnet)
			}
		}
	}
	return nil
}

func (obj *DockerNetworkAddressRes) Default() engine.Res {
	return &DockerNetworkAddressRes{}
}

func (obj *DockerNetworkAddressRes) Init(init *engine.Init) error {
	obj.init = init
	if !obj.IsGrouped() {
		return fmt.Errorf("must be grouped with a docker:container")
	}
	return nil
}

// Close has no function for DockerContainerMount resources
func (obj *DockerNetworkAddressRes) Close() error {
	return nil
}

// Watch has no function for DockerContainerMount resources
func (obj *DockerNetworkAddressRes) Watch() error {
	obj.init.Running()
	<-obj.init.Done
	return nil
}

// CheckApply has no function for auto-grouped child resources
func (obj *DockerNetworkAddressRes) CheckApply(apply bool) (bool, error) {
	return true, fmt.Errorf("resource %s cannot CheckApply", obj)
}

func (obj *DockerNetworkAddressRes) Cmp(r engine.Res) error {
	// we can only compare DockerNetworkAddressRes to others of the same resource kind
	res, ok := r.(*DockerNetworkAddressRes)
	if !ok {
		return fmt.Errorf("error casting r to *DockerNetworkAddressRes")
	}
	if obj.Subnet != res.Subnet {
		return fmt.Errorf("the State differs")
	}
	if obj.IPRange != res.IPRange {
		return fmt.Errorf("the Force differs")
	}
	if obj.Gateway != res.Gateway {
		return fmt.Errorf("the Force differs")
	}
	if !reflect.DeepEqual(obj.AuxAddress, res.AuxAddress) {
		return fmt.Errorf("the AuxAddresses differ")
	}
	return nil
}

func (obj *DockerNetworkAddressRes) IPAMConfig() network.IPAMConfig {
	return network.IPAMConfig{
		Subnet:     obj.Subnet,
		IPRange:    obj.IPRange,
		Gateway:    obj.Gateway,
		AuxAddress: obj.AuxAddress,
	}
}

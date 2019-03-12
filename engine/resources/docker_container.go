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

// +build !nodocker

package resources

import (
	"context"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	// ContainerRunning is the running container state.
	ContainerRunning = "running"
	// ContainerStopped is the stopped container state.
	ContainerStopped = "stopped"
	// ContainerRemoved is the removed container state.
	ContainerRemoved = "removed"

	// initCtxTimeout is the length of time, in seconds, before requests are
	// cancelled in Init.
	initCtxTimeout = 20
	// checkApplyCtxTimeout is the length of time, in seconds, before requests
	// are cancelled in CheckApply.
	checkApplyCtxTimeout = 120
)

func init() {
	engine.RegisterResource("docker:container", func() engine.Res { return &DockerContainerRes{} })
}

// DockerContainerRes is a docker container resource.
type DockerContainerRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	// State of the container must be running, stopped, or removed.
	State string `yaml:"state"`
	// Image is a docker image, or image:tag.
	Image string `yaml:"image"`
	// Cmd is a command, or list of commands to run on the container.
	Cmd []string `yaml:"cmd"`
	// Env is a list of environment variables. E.g. ["VAR=val",].
	Env []string `yaml:"env"`
	// Ports is a map of port bindings. E.g. {"tcp" => {80 => 8080},}.
	Ports map[string]map[int64]int64 `yaml:"ports"`
	// APIVersion allows you to override the host's default client API version.
	APIVersion string `yaml:"apiversion"`

	// Force, if true, will destroy and redeploy the container if the image is
	// incorrect.
	Force bool `yaml:"force"`

	client *client.Client // docker api client

	init *engine.Init
}

// Default returns some sensible defaults for this resource.
func (obj *DockerContainerRes) Default() engine.Res {
	return &DockerContainerRes{}
}

// Validate if the params passed in are valid data.
func (obj *DockerContainerRes) Validate() error {
	// validate state
	if obj.State != ContainerRunning && obj.State != ContainerStopped && obj.State != ContainerRemoved {
		return fmt.Errorf("state must be running, stopped or removed")
	}

	// validate env
	for _, env := range obj.Env {
		if !strings.Contains(env, "=") || strings.Contains(env, " ") {
			return fmt.Errorf("invalid environment variable: %s", env)
		}
	}

	// validate ports
	for k, v := range obj.Ports {
		if k != "tcp" && k != "udp" && k != "sctp" {
			return fmt.Errorf("ports primary key should be tcp, udp or sctp")
		}
		for p, q := range v {
			if (p < 1 || p > 65535) || (q < 1 || q > 65535) {
				return fmt.Errorf("ports must be between 1 and 65535")
			}
		}
	}

	// validate APIVersion
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
func (obj *DockerContainerRes) Init(init *engine.Init) error {
	var err error
	obj.init = init // save for later

	ctx, cancel := context.WithTimeout(context.Background(), initCtxTimeout*time.Second)
	defer cancel()

	// Initialize the docker client.
	obj.client, err = client.NewClient(client.DefaultDockerHost, obj.APIVersion, nil, nil)
	if err != nil {
		return errwrap.Wrapf(err, "error creating docker client")
	}

	// Validate the image.
	resp, err := obj.client.ImageSearch(ctx, obj.Image, types.ImageSearchOptions{Limit: 1})
	if err != nil {
		return errwrap.Wrapf(err, "error searching for image")
	}
	if len(resp) == 0 {
		return fmt.Errorf("image: %s not found", obj.Image)
	}
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *DockerContainerRes) Close() error {
	return obj.client.Close() // close the docker client
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DockerContainerRes) Watch() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan, errChan := obj.client.Events(ctx, types.EventsOptions{})

	obj.init.Running() // when started, notify engine that we're running

	var send = false // send event?
	for {
		select {
		case event, ok := <-eventChan:
			if !ok { // channel shutdown
				return nil
			}
			if obj.init.Debug {
				obj.init.Logf("%+v", event)
			}
			send = true

		case err, ok := <-errChan:
			if !ok {
				return nil
			}
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
func (obj *DockerContainerRes) CheckApply(apply bool) (checkOK bool, err error) {
	var id string
	var destroy bool

	ctx, cancel := context.WithTimeout(context.Background(), checkApplyCtxTimeout*time.Second)
	defer cancel()

	// List any container whose name matches this resource.
	opts := types.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "name", Value: obj.Name()}),
	}
	containerList, err := obj.client.ContainerList(ctx, opts)
	if err != nil {
		return false, errwrap.Wrapf(err, "error listing containers")
	}

	if len(containerList) > 1 {
		return false, fmt.Errorf("more than one container named %s", obj.Name())
	}
	if len(containerList) == 0 && obj.State == ContainerRemoved {
		return true, nil
	}
	if len(containerList) == 1 {
		// If the state and image are correct, we're done.
		if containerList[0].State == obj.State && containerList[0].Image == obj.Image {
			return true, nil
		}
		id = containerList[0].ID // save the id for later
		// If the image is wrong, and force is true, mark the container for
		// destruction.
		if containerList[0].Image != obj.Image && obj.Force {
			destroy = true
		}
		// Otherwise return an error.
		if containerList[0].Image != obj.Image && !obj.Force {
			return false, fmt.Errorf("%s exists but has the wrong image: %s", obj.Name(), containerList[0].Image)

		}
	}

	if !apply {
		return false, nil
	}

	if obj.State == ContainerStopped { // container exists and should be stopped
		return false, obj.containerStop(ctx, id, nil)
	}

	if obj.State == ContainerRemoved { // container exists and should be removed
		if err := obj.containerStop(ctx, id, nil); err != nil {
			return false, err
		}
		return false, obj.containerRemove(ctx, id, types.ContainerRemoveOptions{})
	}

	if destroy {
		if err := obj.containerStop(ctx, id, nil); err != nil {
			return false, err
		}
		if err := obj.containerRemove(ctx, id, types.ContainerRemoveOptions{}); err != nil {
			return false, err
		}
		containerList = []types.Container{} // zero the list
	}

	if len(containerList) == 0 { // no container was found
		// Download the specified image if it doesn't exist locally.
		p, err := obj.client.ImagePull(ctx, obj.Image, types.ImagePullOptions{})
		if err != nil {
			return false, errwrap.Wrapf(err, "error pulling image")
		}
		// Wait for the image to download, EOF signals that it's done.
		if _, err := ioutil.ReadAll(p); err != nil {
			return false, errwrap.Wrapf(err, "error reading image pull result")
		}

		// set up port bindings
		containerConfig := &container.Config{
			Image:        obj.Image,
			Cmd:          obj.Cmd,
			Env:          obj.Env,
			ExposedPorts: make(map[nat.Port]struct{}),
		}

		hostConfig := &container.HostConfig{
			PortBindings: make(map[nat.Port][]nat.PortBinding),
		}

		for k, v := range obj.Ports {
			for p, q := range v {
				containerConfig.ExposedPorts[nat.Port(k)] = struct{}{}
				hostConfig.PortBindings[nat.Port(fmt.Sprintf("%d/%s", p, k))] = []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: fmt.Sprintf("%d", q),
					},
				}
			}
		}

		c, err := obj.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, obj.Name())
		if err != nil {
			return false, errwrap.Wrapf(err, "error creating container")
		}
		id = c.ID
	}

	return false, obj.containerStart(ctx, id, types.ContainerStartOptions{})
}

// containerStart starts the specified container, and waits for it to start.
func (obj *DockerContainerRes) containerStart(ctx context.Context, id string, opts types.ContainerStartOptions) error {
	// Get an events channel for the container we're about to start.
	eventOpts := types.EventsOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "container", Value: id}),
	}
	eventCh, errCh := obj.client.Events(ctx, eventOpts)
	// Start the container.
	if err := obj.client.ContainerStart(ctx, id, opts); err != nil {
		return errwrap.Wrapf(err, "error starting container")
	}
	// Wait for a message on eventChan that says the container has started.
	select {
	case event := <-eventCh:
		if event.Status != "start" {
			return fmt.Errorf("unexpected event: %+v", event)
		}
	case err := <-errCh:
		return errwrap.Wrapf(err, "error waiting for container start")
	}
	return nil
}

// containerStop stops the specified container and waits for it to stop.
func (obj *DockerContainerRes) containerStop(ctx context.Context, id string, timeout *time.Duration) error {
	ch, errCh := obj.client.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	obj.client.ContainerStop(ctx, id, timeout)
	select {
	case <-ch:
	case err := <-errCh:
		return errwrap.Wrapf(err, "error waiting for container to stop")
	}
	return nil
}

// containerRemove removes the specified container and waits for it to be
// removed.
func (obj *DockerContainerRes) containerRemove(ctx context.Context, id string, opts types.ContainerRemoveOptions) error {
	ch, errCh := obj.client.ContainerWait(ctx, id, container.WaitConditionRemoved)
	obj.client.ContainerRemove(ctx, id, opts)
	select {
	case <-ch:
	case err := <-errCh:
		return errwrap.Wrapf(err, "error waiting for container to be removed")
	}
	return nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DockerContainerRes) Cmp(r engine.Res) error {
	// we can only compare DockerContainerRes to others of the same resource kind
	res, ok := r.(*DockerContainerRes)
	if !ok {
		return fmt.Errorf("error casting r to *DockerContainerRes")
	}
	if obj.Name() != res.Name() {
		return fmt.Errorf("names differ")
	}
	if err := util.SortedStrSliceCompare(obj.Cmd, res.Cmd); err != nil {
		return errwrap.Wrapf(err, "cmd differs")
	}
	if err := util.SortedStrSliceCompare(obj.Env, res.Env); err != nil {
		return errwrap.Wrapf(err, "env differs")
	}
	if len(obj.Ports) != len(res.Ports) {
		return fmt.Errorf("ports length differs")
	}
	for k, v := range obj.Ports {
		for p, q := range v {
			if w, ok := res.Ports[k][p]; !ok || q != w {
				return fmt.Errorf("ports differ")
			}
		}
	}
	if obj.APIVersion != res.APIVersion {
		return fmt.Errorf("apiversions differ")
	}
	if obj.Force != res.Force {
		return fmt.Errorf("forces differ")
	}
	return nil
}

// DockerUID is the UID struct for DockerContainerRes.
type DockerUID struct {
	engine.BaseUID
	name string
}

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
func (obj *DockerContainerRes) UIDs() []engine.ResUID {
	x := &DockerUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
func (obj *DockerContainerRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DockerContainerRes // indirection to avoid infinite recursion

	def := obj.Default()                 // get the default
	res, ok := def.(*DockerContainerRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DockerContainerRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DockerContainerRes(raw) // restore from indirection with type conversion!
	return nil
}

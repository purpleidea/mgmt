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

//go:build !nodocker

package resources

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerEvents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	dockerImage "github.com/docker/docker/api/types/image"
	dockerClient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	// ContainerRunning is the running container state.
	ContainerRunning = "running"
	// ContainerStopped is the stopped container state.
	ContainerStopped = "stopped"
	// ContainerRemoved is the removed container state.
	ContainerRemoved = "removed"
)

func init() {
	engine.RegisterResource("docker:container", func() engine.Res { return &DockerContainerRes{} })
}

// DockerContainerRes is a docker container resource.
type DockerContainerRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	// State of the container must be running, stopped, or removed.
	State string `lang:"state" yaml:"state"`

	// Image is a docker image, or image:tag.
	Image string `lang:"image" yaml:"image"`

	// Cmd is a command, or list of commands to run on the container.
	Cmd []string `lang:"cmd" yaml:"cmd"`

	// Env is a list of environment variables. E.g. ["VAR=val",].
	Env []string `lang:"env" yaml:"env"`

	// Ports is a map of port bindings. E.g. {"tcp" => {8080 => 80},}. The
	// key is the host port, and the val is the inner service port to
	// forward to.
	Ports map[string]map[int64]int64 `lang:"ports" yaml:"ports"`

	// APIVersion allows you to override the host's default client API
	// version.
	APIVersion string `lang:"apiversion" yaml:"apiversion"`

	// Force, if true, this will destroy and redeploy the container if the
	// image is incorrect.
	Force bool `lang:"force" yaml:"force"`

	init *engine.Init

	client *dockerClient.Client // docker api client

	once  *sync.Once
	start chan struct{} // closes by once
	sflag bool          // first time happened?
	ready chan struct{} // closes by once
}

// Default returns some sensible defaults for this resource.
func (obj *DockerContainerRes) Default() engine.Res {
	return &DockerContainerRes{
		State: "running",
	}
}

// Validate if the params passed in are valid data.
func (obj *DockerContainerRes) Validate() error {
	// validate state
	if obj.State != ContainerRunning && obj.State != ContainerStopped && obj.State != ContainerRemoved {
		return fmt.Errorf("state must be running, stopped or removed")
	}

	// make sure an image is specified
	if obj.Image == "" {
		return fmt.Errorf("image must be specified")
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
	obj.init = init // save for later

	obj.once = &sync.Once{}
	obj.start = make(chan struct{})
	obj.ready = make(chan struct{})

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DockerContainerRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DockerContainerRes) Watch(ctx context.Context) error {
	var client *dockerClient.Client
	var err error

	for {
		client, err = dockerClient.NewClientWithOpts(dockerClient.WithVersion(obj.APIVersion))
		if err == nil {
			// the above won't check the connection, force that here
			_, err = client.Ping(ctx)
		}
		if err == nil {
			break
		}
		// If we didn't connect right away, it might be because we're
		// waiting for someone to install the docker package, and start
		// the service. We might even have an edge between this resource
		// and those dependencies, but that doesn't stop this Watch from
		// starting up. As a result, we will wait *once* for CheckApply
		// to unlock us, since that runs in dependency order.
		// This error looks like: Cannot connect to the Docker daemon at
		// unix:///var/run/docker.sock. Is the docker daemon running?
		if dockerClient.IsErrConnectionFailed(err) && !obj.sflag {
			// notify engine that we're running so that CheckApply
			// can start...
			obj.init.Running()
			select {
			case <-obj.start:
				obj.sflag = true
				continue

			case <-ctx.Done(): // don't block
				close(obj.ready) // tell CheckApply to unblock!
				return nil
			}
		}
		close(obj.ready) // tell CheckApply to unblock!
		return errwrap.Wrapf(err, "error creating docker client")
	}
	defer client.Close() // success, so close it later

	eventChan, errChan := client.Events(ctx, types.EventsOptions{})
	close(obj.ready) // tell CheckApply to start now that events are running

	// notify engine that we're running
	if !obj.sflag {
		obj.init.Running()
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok { // channel shutdown
				return nil
			}
			if obj.init.Debug {
				obj.init.Logf("%+v", event)
			}

		case err, ok := <-errChan:
			if !ok {
				return nil
			}
			return err

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		obj.init.Event() // notify engine of an event (this can block)
	}
}

// CheckApply method for Docker resource.
func (obj *DockerContainerRes) CheckApply(ctx context.Context, apply bool) (bool, error) {

	obj.once.Do(func() { close(obj.start) }) // Tell Watch() it's safe to start again.
	// Now wait to make sure events are started before we make changes!
	select {
	case <-obj.ready:
	case <-ctx.Done(): // don't block
		return false, ctx.Err()
	}

	var id string
	var destroy bool
	var err error

	// Initialize the docker client.
	obj.client, err = dockerClient.NewClientWithOpts(dockerClient.WithVersion(obj.APIVersion))
	if err != nil {
		return false, errwrap.Wrapf(err, "error creating docker client")
	}
	defer obj.client.Close() // close the docker client

	// Validate the image.
	resp, err := obj.client.ImageSearch(ctx, obj.Image, types.ImageSearchOptions{Limit: 1})
	if err != nil {
		return false, errwrap.Wrapf(err, "error searching for image")
	}
	if len(resp) == 0 {
		return false, fmt.Errorf("image: %s not found", obj.Image)
	}

	// List any container whose name matches this resource.
	opts := container.ListOptions{
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
	// NOTE: If container doesn't exist, we might as well accept "stopped"
	// as valid for now, at least until we rewrite this horrible code.
	if len(containerList) == 0 && (obj.State == ContainerRemoved || obj.State == ContainerStopped) {
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

	// XXX: Check if defined ports matches what we expect.

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
		return false, obj.containerRemove(ctx, id, container.RemoveOptions{})
	}

	if destroy {
		if err := obj.containerStop(ctx, id, nil); err != nil {
			return false, err
		}
		if err := obj.containerRemove(ctx, id, container.RemoveOptions{}); err != nil {
			return false, err
		}
		containerList = []types.Container{} // zero the list
	}

	if len(containerList) == 0 { // no container was found
		// Download the specified image if it doesn't exist locally.
		p, err := obj.client.ImagePull(ctx, obj.Image, dockerImage.PullOptions{})
		if err != nil {
			return false, errwrap.Wrapf(err, "error pulling image")
		}
		// Wait for the image to download, EOF signals that it's done.
		if _, err := io.ReadAll(p); err != nil {
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

		for proto, v := range obj.Ports {
			// On the outside, on the host, we'd see 8080 which is p
			// and on the inside, the container would have something
			// running on 80, which is q.
			for p, q := range v {
				// Port is a string containing port number and
				// protocol in the format "80/tcp".
				port := fmt.Sprintf("%d/%s", q, proto)
				n := nat.Port(port)
				containerConfig.ExposedPorts[n] = struct{}{} // PortSet

				pb := nat.PortBinding{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", p), // eg: 8080
				}
				if _, exists := hostConfig.PortBindings[n]; !exists {
					hostConfig.PortBindings[n] = []nat.PortBinding{}
				}
				hostConfig.PortBindings[n] = append(hostConfig.PortBindings[n], pb)
			}
		}

		c, err := obj.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, obj.Name())
		if err != nil {
			return false, errwrap.Wrapf(err, "error creating container")
		}
		id = c.ID
	}

	return false, obj.containerStart(ctx, id, container.StartOptions{})
}

// containerStart starts the specified container, and waits for it to start.
func (obj *DockerContainerRes) containerStart(ctx context.Context, id string, opts container.StartOptions) error {
	obj.init.Logf("starting...")
	// Get an events channel for the container we're about to start.
	eventOpts := types.EventsOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{Key: "container", Value: id}),
	}
	ch, errCh := obj.client.Events(ctx, eventOpts)
	// Start the container.
	if err := obj.client.ContainerStart(ctx, id, opts); err != nil {
		return errwrap.Wrapf(err, "error starting container")
	}
	// Wait for a message on eventChan that says the container has started.
	// TODO: Should we add ctx here or does cancelling above guarantee exit?
	event, err := dualChannelWaitEvent(ctx, ch, errCh)
	if err != nil {
		return errwrap.Wrapf(err, "error waiting for container start")
	}
	if event.Status != "start" {
		return fmt.Errorf("unexpected event: %+v", event)
	}
	return nil
}

// containerStop stops the specified container and waits for it to stop.
func (obj *DockerContainerRes) containerStop(ctx context.Context, id string, timeout *int) error {
	obj.init.Logf("stopping...")
	ch, errCh := obj.client.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	stopOpts := container.StopOptions{
		Timeout: timeout,
	}
	obj.client.ContainerStop(ctx, id, stopOpts)
	// TODO: Should we add ctx here or does cancelling above guarantee exit?
	if err := dualChannelWait(ctx, ch, errCh); err != nil {
		return errwrap.Wrapf(err, "error waiting for container start")
	}
	return nil
}

// containerRemove removes the specified container and waits for it to be
// removed.
func (obj *DockerContainerRes) containerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	obj.init.Logf("removing...")
	ch, errCh := obj.client.ContainerWait(ctx, id, container.WaitConditionRemoved)
	obj.client.ContainerRemove(ctx, id, opts)
	// TODO: Should we add ctx here or does cancelling above guarantee exit?
	if err := dualChannelWait(ctx, ch, errCh); err != nil {
		return errwrap.Wrapf(err, "error waiting for container start")
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

	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}
	if obj.Image != res.Image {
		return fmt.Errorf("the Image differs")
	}
	if err := util.SortedStrSliceCompare(obj.Cmd, res.Cmd); err != nil {
		return errwrap.Wrapf(err, "the Cmd field differs")
	}
	if err := util.SortedStrSliceCompare(obj.Env, res.Env); err != nil {
		return errwrap.Wrapf(err, "the Env field differs")
	}
	if len(obj.Ports) != len(res.Ports) {
		return fmt.Errorf("the Ports length differs")
	}
	for k, v := range obj.Ports {
		for p, q := range v {
			if w, ok := res.Ports[k][p]; !ok || q != w {
				return fmt.Errorf("the Ports field differs")
			}
		}
	}
	if obj.APIVersion != res.APIVersion {
		return fmt.Errorf("the APIVersion differs")
	}
	if obj.Force != res.Force {
		return fmt.Errorf("the Force field differs")
	}
	return nil
}

// DockerContainerUID is the UID struct for DockerContainerRes.
type DockerContainerUID struct {
	engine.BaseUID

	name string
}

// DockerContainerResAutoEdges holds the state of the auto edge generator.
type DockerContainerResAutoEdges struct {
	UIDs    []engine.ResUID
	pointer int
}

// AutoEdges returns edges to any docker:image resource that matches the image
// specified in the docker:container resource definition.
func (obj *DockerContainerRes) AutoEdges() (engine.AutoEdge, error) {
	var result []engine.ResUID
	var reversed bool
	if obj.State != "removed" {
		reversed = true
	}
	result = append(result, &DockerImageUID{
		BaseUID: engine.BaseUID{
			Reversed: &reversed,
		},
		image: dockerImageNameTag(obj.Image),
	})
	return &DockerContainerResAutoEdges{
		UIDs:    result,
		pointer: 0,
	}, nil
}

// Next returns the next automatic edge.
func (obj *DockerContainerResAutoEdges) Next() []engine.ResUID {
	if len(obj.UIDs) == 0 {
		return nil
	}
	value := obj.UIDs[obj.pointer]
	obj.pointer++
	return []engine.ResUID{value}
}

// Test gets results of the earlier Next() call, & returns if we should
// continue.
func (obj *DockerContainerResAutoEdges) Test(input []bool) bool {
	if len(obj.UIDs) <= obj.pointer {
		return false
	}
	if len(input) != 1 { // in case we get given bad data
		panic(fmt.Sprintf("Expecting a single value!"))
	}
	return true // keep going
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *DockerContainerRes) UIDs() []engine.ResUID {
	x := &DockerContainerUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		name:    obj.Name(),
	}
	return []engine.ResUID{x}
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
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

// dualChannelWait is a helper to wrap the horrendous upstream API.
func dualChannelWait(ctx context.Context, chEvent <-chan container.WaitResponse, chError <-chan error) error {
	var ev container.WaitResponse
	var err error
	ok1, ok2 := true, true

	for {
		if !ok1 && !ok2 {
			// programming error with library's channel API
			return fmt.Errorf("both channels closed")
		}
		select {
		case ev, ok1 = <-chEvent:
			if !ok1 {
				// wait on chError
				continue
			}
			// We might also pass through a nil here if ev.Error is.
			if ev.Error == nil {
				return nil
			}
			return fmt.Errorf("response contained an error: %s", ev.Error.Message)

		case err, ok2 = <-chError:
			if !ok2 {
				// wait on chEvent
				continue
			}
			return errwrap.Wrapf(err, "error waiting for container start")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// dualChannelWaitEvent is a helper to wrap the horrendous upstream API.
func dualChannelWaitEvent(ctx context.Context, chEvent <-chan dockerEvents.Message, chError <-chan error) (*dockerEvents.Message, error) {
	var ev dockerEvents.Message
	var err error
	ok1, ok2 := true, true

	for {
		if !ok1 && !ok2 {
			// programming error with library's channel API
			return nil, fmt.Errorf("both channels closed")
		}
		select {
		case ev, ok1 = <-chEvent:
			if !ok1 {
				// wait on chError
				continue
			}
			return &ev, nil
		case err, ok2 = <-chError:
			if !ok2 {
				// wait on chEvent
				continue
			}
			return nil, errwrap.Wrapf(err, "error waiting for container start")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

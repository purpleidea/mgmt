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

// +build !nodocker

package resources

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
	"time"

	"github.com/docker/docker/api/types/network"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	dockeropts "github.com/docker/cli/opts"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

const (
	// ContainerRunning is the running container state.
	ContainerRunning = "running"
	// ContainerStopped is the stopped container state.
	ContainerStopped = "stopped"
	// ContainerAbsent is the absent container state.
	ContainerAbsent = "absent"

	// initCtxTimeout is the length of time, in seconds, before requests are
	// cancelled in Init.
	initCtxTimeout = 20
	// checkApplyCtxTimeout is the length of time, in seconds, before
	// requests are cancelled in CheckApply.
	checkApplyCtxTimeout = 120
)

func init() {
	engine.RegisterResource("docker:container", func() engine.Res { return &DockerContainerRes{} })
}

// DockerContainerRes is a docker container resource.
type DockerContainerRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable
	traits.Groupable

	// State of the container must be running, stopped, or absent.
	State string `lang:"state"`

	// Networks
	Networks []string `lang:"networks"`

	NetworkMode string `lang:"network_mode"`

	// Entrypoint
	Entrypoint []string `lang:"entrypoint"`
	// Cmd is a command, or list of commands to run on the container.
	Cmd []string `lang:"cmd"`
	// DNS is a list of custom DNS servers
	DNS []string `lang:"dns"`
	// Devices is a list of device mappings
	Devices []string `lang:"devices"`
	// Domainname is the Domain name of the container
	Domainname *string `lang:"domainname"`
	// Env is a map of environment variables. E.g. {"VAR" => "val",}.
	Env map[string]string `lang:"env"`
	// Hostname is the hostname of the container
	Hostname *string `lang:"hostname"`
	// Image is a docker image, or image:tag.
	Image string `lang:"image"`
	// Labels is a list of metadata labels
	Labels map[string]string `lang:"labels"`
	// Ports is a map of port bindings. E.g. {"tcp" => {80 => "1.2.3.4:8080"},}.
	Ports map[string]map[int64]string `lang:"ports"`
	// portSet
	portSet nat.PortSet
	// portMap
	portMap nat.PortMap
	// Restart is the policy used to determine how to restart the container
	Restart *string `lang:"restart"`
	// parsed restart policy and assigned during Validate()
	restartPolicy container.RestartPolicy
	// User is the username/uid that will run the cmd inside the container
	User *string `lang:"user"`
	// WorkingDir is the working directory of the container init process
	WorkingDir *string `lang:"workdir"`

	// APIVersion allows you to override the host's default client API
	// version.
	APIVersion string `lang:"apiversion"`

	// Force, if true, this will destroy and redeploy the container if the
	// image is incorrect.
	Force bool `lang:"force"`

	client *client.Client // docker api client

	init *engine.Init
}

// Default returns some sensible defaults for this resource.
func (obj *DockerContainerRes) Default() engine.Res {
	return &DockerContainerRes{
		State: "running",
	}
}

// Validate if the params passed in are valid data.
func (obj DockerContainerRes) Validate() error {
	// validate state
	if obj.State != ContainerRunning && obj.State != ContainerStopped && obj.State != ContainerAbsent {
		return fmt.Errorf("state must be running, stopped or absent")
	}

	// make sure an image is specified
	if obj.Image == "" {
		return fmt.Errorf("image must be specified")
	}

	// validate env
	for key := range obj.Env {
		if key == "" {
			return fmt.Errorf("environment variable name cannot be empty")
		}
	}

	// validate ports
	var portSpecs []string
	for proto, mapping := range obj.Ports {
		if proto != "tcp" && proto != "udp" && proto != "sctp" {
			return fmt.Errorf("ports primary key should be tcp, udp or sctp")
		}
		for ctr, host := range mapping {
			if ctr < 1 || ctr > 65535 {
				return fmt.Errorf("ports must be between 1 and 65535")
			}
			portSpecs = append(portSpecs, fmt.Sprintf("%s:%d/%s", host, ctr, proto))
		}
	}
	var err error
	//obj.portSet, obj.portMap, err = nat.ParsePortSpecs(portSpecs)
	_, _, err = nat.ParsePortSpecs(portSpecs)
	// FIXME(frebib): populate portSet and portMap inside Init()
	if err != nil {
		// programming error; should be caught in validate
		return errwrap.Wrapf(err, "port bindings invalid")
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

	// validate restart policy
	if obj.Restart != nil {
		policy, err := dockeropts.ParseRestartPolicy(*obj.Restart)
		if err != nil {
			return fmt.Errorf("invalid restart policy: %s", err)
		}
		if !(policy.IsAlways() || policy.IsNone() ||
			policy.IsOnFailure() || policy.IsUnlessStopped()) {
			return fmt.Errorf("restart-policy must be always, on-failure, unless-stopped or no")
		}
		obj.restartPolicy = container.RestartPolicy(policy)
	}

	// validate working directory
	if obj.WorkingDir != nil {
		workdir := *obj.WorkingDir
		if workdir == "" {
			return errors.New("working directory cannot be empty")
		}
		if workdir[0:1] != "/" {
			return errors.New("working directory must be absolute")
		}
	}

	// validate network mode
	if obj.NetworkMode != "" && obj.NetworkMode != "none" &&
		obj.NetworkMode != "bridge" && obj.NetworkMode != "host" {
		//return errors.New("network mode must be one of none, bridge or host")
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
	obj.client, err = client.NewClientWithOpts(client.WithVersion(obj.APIVersion))
	if err != nil {
		return errwrap.Wrapf(err, "error creating docker client")
	}

	// Validate the image.
	resp, err := obj.client.ImageSearch(ctx, obj.Image, types.ImageSearchOptions{Limit: 1})
	if err != nil {
		obj.init.Logf(errwrap.Wrapf(err, "error searching for image").Error())
		//return errwrap.Wrapf(err, "error searching for image")
	}
	if len(resp) == 0 {
		obj.init.Logf("image: %s not found", obj.Image)
		//return fmt.Errorf("image: %s not found", obj.Image)
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
		case _, ok := <-eventChan:
			if !ok { // channel shutdown
				return nil
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
func (obj *DockerContainerRes) CheckApply(apply bool) (bool, error) {
	var ctr types.ContainerJSON
	var destroy bool

	ctx, cancel := context.WithTimeout(context.Background(), checkApplyCtxTimeout*time.Second)
	defer cancel()

	// List any container whose name matches this resource.
	opts := types.ContainerListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", obj.Name())),
	}
	containerList, err := obj.client.ContainerList(ctx, opts)
	if err != nil {
		return false, errwrap.Wrapf(err, "error listing containers")
	}

	// this should never happen
	if len(containerList) > 1 {
		return false, fmt.Errorf("more than one container named %s", obj.Name())
	}
	// handle ContainerAbsent here else it'll cause containerStop to error
	if len(containerList) == 0 &&
		(obj.State == ContainerStopped || obj.State == ContainerAbsent) {
		return true, nil
	}

	if len(containerList) == 1 {
		// inspect the container for all the gory volume/network details
		ctr, err = obj.client.ContainerInspect(ctx, containerList[0].ID)
		if err != nil {
			return false, errwrap.Wrapf(err, "error inspecting container: %s", containerList[0].ID)
		}

		// Check first all properties that require the container to be recreated
		// in case we pointlessly change some properties, but need to destroy
		// the container afterwards and recreate it anyway.
		if err := obj.CmpContainer(ctx, ctr); err != nil {
			obj.init.Logf(err.Error())
			destroy = true
		}

		// only update the container when it's in the correct state
		if !destroy && obj.State == ctr.State.Status {
			// Final checks are to ensure all updatable configurables are
			// correct and don't need to be changed.
			return obj.containerUpdate(ctx, ctr.ID, apply)
		}

		// Return an error if the running state does not match and it cannot
		// be updated without destroying the container.
		if destroy && !obj.Force {
			return false, fmt.Errorf("%s exists but the config does not match", obj.Name())
		}
	}

	if !apply { // do nothing and inform whether we would have done something
		return !destroy, nil
	}

	if obj.State == ContainerStopped { // container exists and should be stopped
		return false, obj.containerStop(ctx, ctr.ID, nil)
	}

	if obj.State == ContainerAbsent { // container exists and should be absent
		if ctr.State.Status == "removing" {
			// Already being removed by Docker. Rare but it can happen
			return false, nil
		}
		return false, obj.containerRemove(ctx, ctr.ID, types.ContainerRemoveOptions{Force: true})
	}

	if destroy {
		options := types.ContainerRemoveOptions{Force: true}
		if err := obj.containerRemove(ctx, ctr.ID, options); err != nil {
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
			Cmd:          obj.Cmd,
			Domainname:   util.StrOrEmpty(obj.Domainname),
			Entrypoint:   obj.Entrypoint,
			Env:          util.StrMapKeyEqualValue(obj.Env),
			ExposedPorts: obj.portSet,
			Hostname:     util.StrOrEmpty(obj.Hostname),
			Image:        obj.Image,
			User:         util.StrOrEmpty(obj.User),
			WorkingDir:   util.StrOrEmpty(obj.WorkingDir),
		}

		hostConfig := &container.HostConfig{
			Mounts:        obj.getMounts(),
			PortBindings:  obj.portMap,
			RestartPolicy: obj.restartPolicy,
			NetworkMode:   container.NetworkMode(obj.NetworkMode),
		}

		networkConfig := &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{},
		}
		for _, name := range obj.Networks {
			networkConfig.EndpointsConfig[name] = &network.EndpointSettings{}
		}

		c, err := obj.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, obj.Name())
		if err != nil {
			return false, errwrap.Wrapf(err, "error creating container")
		}

		ctr, err = obj.client.ContainerInspect(ctx, c.ID)
		if err != nil {
			return false, errwrap.Wrapf(err, "error inspecting container: %s", containerList[0].ID)
		}
	}

	if ctr.State.Status == "restarting" || ctr.State.Status == "removing" {
		// Docker will restart the container on it's own, or remove it shortly
		return false, nil
	}

	return false, obj.containerStart(ctx, ctr.ID, types.ContainerStartOptions{})
}

// containerStart starts the specified container, and waits for it to start.
func (obj *DockerContainerRes) containerStart(ctx context.Context, id string, opts types.ContainerStartOptions) error {
	// Get an events channel for the container we're about to start.
	eventOpts := types.EventsOptions{
		Filters: filters.NewArgs(filters.Arg("container", id)),
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

func (obj *DockerContainerRes) containerUpdate(ctx context.Context, id string, apply bool) (bool, error) {
	changed := false

	ctr, err := obj.client.ContainerInspect(ctx, id)
	if err != nil {
		return false, errwrap.Wrapf(err, "error inspecting container: %s", id)
	}

	// check properties that can be changed at runtime
	if obj.restartPolicy.Name != "" && !ctr.HostConfig.RestartPolicy.IsSame(&obj.restartPolicy) {
		if !apply {
			return false, nil
		}
		obj.init.Logf("updating restart policy")
		_, err = obj.client.ContainerUpdate(ctx, id, container.UpdateConfig{
			RestartPolicy: obj.restartPolicy,
		})
		if err != nil {
			return false, errwrap.Wrapf(err, "failed to update restart policy")
		}
		changed = true
	}

	return !changed, nil
}

func (obj *DockerContainerRes) getMounts() []mount.Mount {
	var mounts []mount.Mount

	for _, res := range obj.GetGroup() {
		mnt, ok := res.(*DockerContainerMountRes) // convert from GroupableRes
		if !ok {
			continue
		}

		mounts = append(mounts, mnt.GetMount())
	}

	return mounts
}

// compareEnv compares the environment of the running container with the obj.Env excluding the environment set in the container backing image returning true if the running container environment matches the obj.Env
func (obj *DockerContainerRes) compareEnv(imgEnv, env []string) bool {
	// []string{key=value} representation of the map[string]string
	objEnv := util.StrMapKeyEqualValue(obj.Env)

	var runningEnv []string
	for _, envVar := range env {
		// skip vars that are specified by the container and also not explicitly
		// specified in the container config
		if util.StrInList(envVar, imgEnv) && !util.StrInList(envVar, objEnv) {
			continue
		}
		runningEnv = append(runningEnv, envVar)
	}

	return util.StrSliceEqual(runningEnv, objEnv)
}

/*
// compareLabels compares the Labels of the running container with the obj.Labels
// excluding the Labels set in the container backing image returning
// true if the running container Labels matches the obj.Labels
func (obj *DockerContainerRes) compareLabels(ctx context.Context, image string, Labels []string) (bool, error) {
	// get the image Labels to remove the vars set in the image
	img, _, err := obj.client.ImageInspectWithRaw(ctx, image)
	if err != nil {
		return false, err
	}

	// []string{key=value} representation of the map[string]string
	objLabels := util.StrMapKeyEqualValue(obj.Labels)

	var runningLabels []string
	for _, LabelsVar := range Labels {
		// skip vars that are specified by the container and also not explicitly
		// specified in the container config
		if util.StrInList(LabelsVar, img.ContainerConfig.Labels) && !util.
			StrInList(LabelsVar, objLabels) {
			continue
		}
		runningLabels = append(runningLabels, LabelsVar)
	}

	return util.StrSliceEqual(runningLabels, objLabels), nil
}
*/

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
	if !util.StrSliceEqual(obj.Cmd, res.Cmd) {
		return fmt.Errorf("the Image differs")
	}
	if obj.Image != res.Image {
		return fmt.Errorf("the Image differs")
	}
	if !reflect.DeepEqual(obj.User, res.User) {
		return fmt.Errorf("the User differs")
	}
	if err := util.SortedStrSliceCompare(obj.Cmd, res.Cmd); err != nil {
		return errwrap.Wrapf(err, "the Cmd differs")
	}
	if err := util.SortedStrSliceCompare(obj.Entrypoint, res.Entrypoint); err != nil {
		return errwrap.Wrapf(err, "the Entrypoint differs")
	}
	if !reflect.DeepEqual(obj.Env, res.Env) {
		return fmt.Errorf("the Env differs")
	}
	if !reflect.DeepEqual(obj.Ports, res.Ports) {
		return fmt.Errorf("the Ports differ")
	}
	if obj.APIVersion != res.APIVersion {
		return fmt.Errorf("the APIVersion differs")
	}
	if obj.Force != res.Force {
		return fmt.Errorf("the Force field differs")
	}
	if !reflect.DeepEqual(obj.WorkingDir, res.WorkingDir) {
		return fmt.Errorf("the WorkingDir field differs")
	}

	return nil
}

// Cmp compares a DockerContainerRes to a types.ContainerJSON and returns an
// error if they're not equivalent
func (obj *DockerContainerRes) CmpContainer(ctx context.Context, ctr types.ContainerJSON) error {
	// get the image Labels to remove the vars set in the image
	img, _, err := obj.client.ImageInspectWithRaw(ctx, ctr.Image)
	if err != nil {
		return errwrap.Wrapf(err, "failed to inspect container image")
	}
	imgCfg := img.ContainerConfig

	// cmpStrResCtrImg returns true if the container is in the correct state
	cmpStrResCtrImg := func(res *string, ctr, img string) bool {
		return (res == nil && ctr == img) || (res != nil && *res == ctr)
	}
	// cmpStrSliceResCtrImg returns true if the container is in the correct state
	cmpStrSliceResCtrImg := func(res, ctr, img []string) bool {
		return (res == nil && util.StrSliceEqual(ctr, img)) ||
			(res != nil && util.StrSliceEqual(res, ctr))
	}

	if obj.Image != ctr.Config.Image {
		return fmt.Errorf("the container Image differs")
	}

	if !cmpStrSliceResCtrImg(obj.Cmd, ctr.Config.Cmd, img.Config.Cmd) {
		return fmt.Errorf("the container Cmd differs")
	}

	if !cmpStrSliceResCtrImg(obj.Entrypoint, ctr.Config.Entrypoint, img.Config.Entrypoint) {
		return fmt.Errorf("the container Entrypoint differs")
	}

	if obj.Hostname != nil && *obj.Hostname != ctr.Config.Hostname {
		return fmt.Errorf("the container Hostname differs")
	}

	if obj.Domainname != nil && *obj.Domainname != ctr.Config.Domainname {
		return fmt.Errorf("the container Domainname differs")
	}

	if !cmpStrResCtrImg(obj.User, ctr.Config.User, imgCfg.User) {
		return fmt.Errorf("the container User differs")
	}

	if !obj.compareEnv(img.Config.Env, ctr.Config.Env) {
		return fmt.Errorf("the container Env differs")
	}

	mounts := obj.getMounts()
	if len(mounts) != len(ctr.HostConfig.Mounts) {
		return fmt.Errorf("the Mount count differs")
	}

	for i := range mounts {
		var mnt *mount.Mount

		// find the matching Mount
		for j := range ctr.HostConfig.Mounts {
			if mounts[i].Target == ctr.HostConfig.Mounts[j].Target {
				mnt = &ctr.HostConfig.Mounts[j]
				break
			}
		}
		// Not found, or found but doesn't match
		if mnt == nil || !reflect.DeepEqual(&mounts[i], mnt) {
			return fmt.Errorf("the container Mounts differ")
		}
	}

	if !cmpStrResCtrImg(obj.WorkingDir, ctr.Config.WorkingDir, img.Config.WorkingDir) {
		return fmt.Errorf("the container WorkingDir differs")
	}

	return nil
}

// GroupCmp returns whether two resources can be grouped together or not. Can
// these two resources be merged, aka, does this resource support doing so? Will
// resource allow itself to be grouped _into_ this obj?
func (obj *DockerContainerRes) GroupCmp(r engine.GroupableRes) error {
	res, ok := r.(*DockerContainerMountRes)
	if ok {
		// group mounts matching this container name
		if res.Container != obj.Name() {
			return fmt.Errorf("resource groups with a different container name")
		}

		return nil
	}

	return fmt.Errorf("resource is not the right kind")
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
	if obj.State != ContainerAbsent {
		reversed = true
	}
	result = append(result, &DockerImageUID{
		BaseUID: engine.BaseUID{
			Reversed: &reversed,
		},
		image: dockerImageNameTag(obj.Image),
	})
	for _, name := range obj.Networks {
		result = append(result, &DockerNetworkUID{
			BaseUID: engine.BaseUID{
				Reversed: &reversed,
			},
			network: name,
		})
	}
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

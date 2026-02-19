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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dockerImage "github.com/docker/docker/api/types/image"
	dockerClient "github.com/docker/docker/client"
	errwrap "github.com/pkg/errors"
)

func init() {
	engine.RegisterResource("docker:image", func() engine.Res { return &DockerImageRes{} })
}

// DockerImageRes is a docker image resource. The resource's name must be a
// docker image in any supported format (url, image, or image:tag).
type DockerImageRes struct {
	traits.Base // add the base methods without re-implementation
	traits.Edgeable

	// State of the image must be exists or absent.
	State string `lang:"state" yaml:"state"`

	// APIVersion allows you to override the host's default client API
	// version.
	APIVersion string `lang:"apiversion" yaml:"apiversion"`

	init *engine.Init

	once  *sync.Once
	start chan struct{} // closes by once
	sflag bool          // first time happened?
	ready chan struct{} // closes by once
}

// Default returns some sensible defaults for this resource.
func (obj *DockerImageRes) Default() engine.Res {
	return &DockerImageRes{
		// TODO: eventually if image supports other properties, this can
		// be left out and we could have the state be "unmanaged".
		State: "exists",
	}
}

// Validate if the params passed in are valid data.
func (obj *DockerImageRes) Validate() error {
	// validate state
	if obj.State != "exists" && obj.State != "absent" {
		return fmt.Errorf("state must be exists or absent")
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
func (obj *DockerImageRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.once = &sync.Once{}
	obj.start = make(chan struct{})
	obj.ready = make(chan struct{})

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *DockerImageRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DockerImageRes) Watch(ctx context.Context) error {
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
			if err := obj.init.Event(ctx); err != nil {
				return err
			}
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
		if err := obj.init.Event(ctx); err != nil {
			return err
		}
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

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply method for Docker resource.
func (obj *DockerImageRes) CheckApply(ctx context.Context, apply bool) (checkOK bool, err error) {

	obj.once.Do(func() { close(obj.start) }) // Tell Watch() it's safe to start again.
	// Now wait to make sure events are started before we make changes!
	select {
	case <-obj.ready:
	case <-ctx.Done(): // don't block
		return false, ctx.Err()
	}

	// Save the full image name and tag.
	image := dockerImageNameTag(obj.Name())

	// Initialize the docker client.
	client, err := dockerClient.NewClientWithOpts(dockerClient.WithVersion(obj.APIVersion))
	if err != nil {
		return false, errwrap.Wrapf(err, "error creating docker client")
	}
	defer client.Close()

	// Validate the image.
	resp, err := client.ImageSearch(ctx, image, types.ImageSearchOptions{Limit: 1})
	if err != nil {
		return false, errwrap.Wrapf(err, "error searching for image")
	}
	if len(resp) == 0 {
		return false, fmt.Errorf("image: %s not found", image)
	}

	s, err := client.ImageList(ctx, dockerImage.ListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", image)),
	})
	if err != nil {
		return false, errwrap.Wrapf(err, "error listing images")
	}
	if len(s) > 1 {
		return false, fmt.Errorf("more than one image found")
	}

	if obj.State == "absent" && len(s) == 0 {
		return true, nil
	}
	if obj.State == "exists" && len(s) == 1 {
		return true, nil
	}

	if !apply {
		return false, nil
	}

	if obj.State == "absent" {
		obj.init.Logf("removing...")
		// TODO: force? prune children?
		if _, err := client.ImageRemove(ctx, image, dockerImage.RemoveOptions{}); err != nil {
			return false, errwrap.Wrapf(err, "error removing image")
		}
		return false, nil
	}

	// pull the image
	obj.init.Logf("pulling...")
	p, err := client.ImagePull(ctx, image, dockerImage.PullOptions{})
	if err != nil {
		return false, errwrap.Wrapf(err, "error pulling image")
	}
	// Wait for the image to download, EOF signals that it's done.
	if _, err := io.ReadAll(p); err != nil {
		return false, errwrap.Wrapf(err, "error reading image pull result")
	}

	return false, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *DockerImageRes) Cmp(r engine.Res) error {
	// we can only compare DockerImageRes to others of the same resource kind
	res, ok := r.(*DockerImageRes)
	if !ok {
		return fmt.Errorf("error casting r to *DockerImageRes")
	}
	if obj.State != res.State {
		return fmt.Errorf("the State differs")
	}

	if obj.APIVersion != res.APIVersion {
		return fmt.Errorf("the APIVersion differs")
	}
	return nil
}

// DockerImageUID is the UID struct for DockerImageRes.
type DockerImageUID struct {
	engine.BaseUID

	image string
}

// UIDs includes all params to make a unique identification of this object. Most
// resources only return one, although some resources can return multiple.
func (obj *DockerImageRes) UIDs() []engine.ResUID {
	x := &DockerImageUID{
		BaseUID: engine.BaseUID{Name: obj.Name(), Kind: obj.Kind()},
		image:   dockerImageNameTag(obj.Name()),
	}
	return []engine.ResUID{x}
}

// AutoEdges returns the AutoEdge interface.
func (obj *DockerImageRes) AutoEdges() (engine.AutoEdge, error) {
	return nil, nil
}

// IFF aka if and only if they are equivalent, return true. If not, false.
func (obj *DockerImageUID) IFF(uid engine.ResUID) bool {
	res, ok := uid.(*DockerImageUID)
	if !ok {
		return false
	}
	return obj.image == res.image
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *DockerImageRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes DockerImageRes // indirection to avoid infinite recursion

	def := obj.Default()             // get the default
	res, ok := def.(*DockerImageRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to DockerImageRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = DockerImageRes(raw) // restore from indirection with type conversion!
	return nil
}

// dockerImageNameTag does a naive check to see if the input includes a tag or
// is a url, and if not, appends the `:latest` tag to ensure disambiguation.
func dockerImageNameTag(image string) string {
	if strings.Contains(image, ":") {
		return image
	}
	return image + ":latest"
}

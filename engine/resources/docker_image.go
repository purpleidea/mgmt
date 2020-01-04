// Mgmt
// Copyright (C) 2013-2020+ James Shubin and the project contributors
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

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	errwrap "github.com/pkg/errors"
)

const (
	// dockerImageInitCtxTimeout is the length of time, in seconds, before
	// requests are cancelled in Init.
	dockerImageInitCtxTimeout = 20
	// dockerImageCheckApplyCtxTimeout is the length of time, in seconds,
	// before requests are cancelled in CheckApply.
	dockerImageCheckApplyCtxTimeout = 120
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
	State string `yaml:"state"`
	// APIVersion allows you to override the host's default client API
	// version.
	APIVersion string `yaml:"apiversion"`

	image  string         // full image:tag format
	client *client.Client // docker api client

	init *engine.Init
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
	var err error
	obj.init = init // save for later

	// Save the full image name and tag.
	obj.image = dockerImageNameTag(obj.Name())

	ctx, cancel := context.WithTimeout(context.Background(), dockerImageInitCtxTimeout*time.Second)
	defer cancel()

	// Initialize the docker client.
	obj.client, err = client.NewClient(client.DefaultDockerHost, obj.APIVersion, nil, nil)
	if err != nil {
		return errwrap.Wrapf(err, "error creating docker client")
	}

	// Validate the image.
	resp, err := obj.client.ImageSearch(ctx, obj.image, types.ImageSearchOptions{Limit: 1})
	if err != nil {
		return errwrap.Wrapf(err, "error searching for image")
	}
	if len(resp) == 0 {
		return fmt.Errorf("image: %s not found", obj.image)
	}
	return nil
}

// Close is run by the engine to clean up after the resource is done.
func (obj *DockerImageRes) Close() error {
	return obj.client.Close() // close the docker client
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *DockerImageRes) Watch() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eventChan, errChan := obj.client.Events(ctx, types.EventsOptions{})

	// notify engine that we're running
	obj.init.Running()

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
func (obj *DockerImageRes) CheckApply(apply bool) (checkOK bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), dockerImageCheckApplyCtxTimeout*time.Second)
	defer cancel()

	s, err := obj.client.ImageList(ctx, types.ImageListOptions{
		Filters: filters.NewArgs(filters.Arg("reference", obj.image)),
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
		// TODO: force? prune children?
		if _, err := obj.client.ImageRemove(ctx, obj.image, types.ImageRemoveOptions{}); err != nil {
			return false, errwrap.Wrapf(err, "error removing image")
		}
		return false, nil
	}

	// pull the image
	p, err := obj.client.ImagePull(ctx, obj.image, types.ImagePullOptions{})
	if err != nil {
		return false, errwrap.Wrapf(err, "error pulling image")
	}
	// Wait for the image to download, EOF signals that it's done.
	if _, err := ioutil.ReadAll(p); err != nil {
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

// UIDs includes all params to make a unique identification of this object.
// Most resources only return one, although some resources can return multiple.
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

// UnmarshalYAML is the custom unmarshal handler for this struct.
// It is primarily useful for setting the defaults.
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

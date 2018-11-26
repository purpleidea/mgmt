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

package resources

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

var res *DockerContainerRes

var id string

func TestMain(m *testing.M) {
	var setupCode, testCode, cleanupCode int

	if err := setup(); err != nil {
		log.Printf("error during setup: %s", err)
		setupCode = 1
	}

	if setupCode == 0 {
		testCode = m.Run()
	}

	if err := cleanup(); err != nil {
		log.Printf("error during cleanup: %s", err)
		cleanupCode = 1
	}

	os.Exit(setupCode + testCode + cleanupCode)
}

func Test_containerStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerStart(ctx, id, types.ContainerStartOptions{}); err != nil {
		t.Errorf("containerStart() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		types.ContainerListOptions{
			Filters: filters.NewArgs(
				filters.KeyValuePair{Key: "id", Value: id},
				filters.KeyValuePair{Key: "status", Value: "running"},
			),
		},
	)
	if err != nil {
		t.Errorf("error listing containers: %s", err)
		return
	}
	if len(l) != 1 {
		t.Errorf("failed to start container")
		return
	}
}

func Test_containerStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerStop(ctx, id, nil); err != nil {
		t.Errorf("containerStop() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		types.ContainerListOptions{
			Filters: filters.NewArgs(
				filters.KeyValuePair{Key: "id", Value: id},
			),
		},
	)
	if err != nil {
		t.Errorf("error listing containers: %s", err)
		return
	}
	if len(l) != 0 {
		t.Errorf("failed to stop container")
		return
	}
}

func Test_containerRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerRemove(ctx, id, types.ContainerRemoveOptions{}); err != nil {
		t.Errorf("containerRemove() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		types.ContainerListOptions{
			All: true,
			Filters: filters.NewArgs(
				filters.KeyValuePair{Key: "id", Value: id},
			),
		},
	)
	if err != nil {
		t.Errorf("error listing containers: %s", err)
		return
	}
	if len(l) != 0 {
		t.Errorf("failed to remove container")
		return
	}
}

func setup() error {
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	res = &DockerContainerRes{}
	res.Init(res.init)

	p, err := res.client.ImagePull(ctx, "alpine", types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("error pulling image: %s", err)
	}
	if _, err := ioutil.ReadAll(p); err != nil {
		return fmt.Errorf("error reading image pull result: %s", err)
	}

	resp, err := res.client.ContainerCreate(
		ctx,
		&container.Config{
			Image: "alpine",
			Cmd:   []string{"sleep", "100"},
		},
		&container.HostConfig{},
		nil,
		"mgmt-test",
	)
	if err != nil {
		return fmt.Errorf("error creating container: %s", err)
	}
	id = resp.ID
	return nil
}

func cleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	l, err := res.client.ContainerList(
		ctx,
		types.ContainerListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.KeyValuePair{Key: "id", Value: id}),
		},
	)
	if err != nil {
		return fmt.Errorf("error listing containers: %s", err)
	}

	if len(l) > 0 {
		if err := res.client.ContainerStop(ctx, id, nil); err != nil {
			return fmt.Errorf("error stopping container: %s", err)
		}
		if err := res.client.ContainerRemove(ctx, id, types.ContainerRemoveOptions{}); err != nil {
			return fmt.Errorf("error removing container: %s", err)
		}
	}
	return nil
}

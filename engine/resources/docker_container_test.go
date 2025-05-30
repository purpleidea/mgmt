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
	"log"
	"os"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
)

var res *DockerContainerRes

var id string

// XXX: re-enable once docker is not broken.
// XXX: Error: docker_container_test.go:112: failed to stop container
func BrokenTestDockerMain(m *testing.M) {
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

func BrokenTestContainerStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerStart(ctx, id, container.StartOptions{}); err != nil {
		t.Errorf("containerStart() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		container.ListOptions{
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

func BrokenTestContainerStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerStop(ctx, id, nil); err != nil {
		t.Errorf("containerStop() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		container.ListOptions{
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

func BrokenTestContainerRemove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := res.containerRemove(ctx, id, container.RemoveOptions{}); err != nil {
		t.Errorf("containerRemove() error: %s", err)
		return
	}

	l, err := res.client.ContainerList(
		ctx,
		container.ListOptions{
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

	p, err := res.client.ImagePull(ctx, "alpine", image.PullOptions{})
	if err != nil {
		return fmt.Errorf("error pulling image: %s", err)
	}
	if _, err := io.ReadAll(p); err != nil {
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
		container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.KeyValuePair{Key: "id", Value: id}),
		},
	)
	if err != nil {
		return fmt.Errorf("error listing containers: %s", err)
	}

	if len(l) > 0 {
		stopOpts := container.StopOptions{}
		if err := res.client.ContainerStop(ctx, id, stopOpts); err != nil {
			return fmt.Errorf("error stopping container: %s", err)
		}
		if err := res.client.ContainerRemove(ctx, id, container.RemoveOptions{}); err != nil {
			return fmt.Errorf("error removing container: %s", err)
		}
	}
	return nil
}

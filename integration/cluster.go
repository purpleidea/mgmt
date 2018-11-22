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

package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
)

// Cluster represents an mgmt cluster. It uses the instance building blocks to
// run clustered tests.
type Cluster struct {
	// Hostnames is the list of unique identifiers for this cluster.
	Hostnames []string

	// Preserve prevents the runtime output from being explicitly deleted.
	// This is helpful for running analysis or tests on the output.
	Preserve bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})

	// Debug enables more verbosity.
	Debug bool

	// dir is the directory where all files will be written under.
	dir string

	instances map[string]*Instance
}

// Init runs some initialization for this Cluster. It errors if the struct was
// populated in an invalid way, or if it can't initialize correctly.
func (obj *Cluster) Init() error {
	obj.instances = make(map[string]*Instance)

	// create temporary directory to use during testing
	var err error
	if obj.dir == "" {
		obj.dir, err = ioutil.TempDir("", "mgmt-integration-cluster-")
		if err != nil {
			return errwrap.Wrapf(err, "can't create temporary directory")
		}
	}

	for _, hostname := range obj.Hostnames {
		h := hostname
		instancePrefix := path.Join(obj.dir, h)
		if err := os.MkdirAll(instancePrefix, dirMode); err != nil {
			return errwrap.Wrapf(err, "can't create instance directory")
		}

		obj.instances[h] = &Instance{
			Hostname: h,
			Preserve: obj.Preserve,
			Logf: func(format string, v ...interface{}) {
				obj.Logf(fmt.Sprintf("instance <%s>: ", h)+format, v...)
			},
			Debug: obj.Debug,

			dir: instancePrefix,
		}
		if e := obj.instances[h].Init(); e != nil {
			err = multierr.Append(err, e)
		}
	}

	return err
}

// Close cleans up after we're done with this Cluster.
func (obj *Cluster) Close() error {
	var err error
	// do this in reverse for fun
	for i := len(obj.Hostnames) - 1; i >= 0; i-- {
		h := obj.Hostnames[i]
		instance, exists := obj.instances[h]
		if !exists {
			continue
		}
		if e := instance.Close(); e != nil {
			err = multierr.Append(err, e)
		}
	}
	if !obj.Preserve {
		if obj.dir == "" || obj.dir == "/" {
			panic("obj.dir is set to a dangerous path")
		}
		if err := os.RemoveAll(obj.dir); err != nil { // dangerous ;)
			return errwrap.Wrapf(err, "can't remove instance dir")
		}
	}
	return err
}

// RunLinear starts up each instance linearly, one at a time.
func (obj *Cluster) RunLinear() error {
	for i, h := range obj.Hostnames {
		// build a list of earlier instances that have already run
		seeds := []*Instance{}
		for j := 0; j < i; j++ {
			x := obj.instances[obj.Hostnames[j]]
			seeds = append(seeds, x)
		}

		instance, exists := obj.instances[h]
		if !exists {
			return fmt.Errorf("instance `%s` not found", h)
		}

		if err := instance.Run(seeds); err != nil {
			return errwrap.Wrapf(err, "trouble running instance `%s`", h)
		}

		// FIXME: consider removing this wait entirely
		// wait for startup before continuing with the next one
		ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		defer cancel()
		if err := instance.Wait(ctx); err != nil { // wait to get a converged signal
			return errwrap.Wrapf(err, "mgmt wait on instance `%s` failed", h) // timeout expired
		}
	}

	return nil
}

// Kill the cluster immediately. This is a `kill -9` for if things get stuck.
func (obj *Cluster) Kill() error {
	var err error
	// do this in reverse for fun
	for i := len(obj.Hostnames) - 1; i >= 0; i-- {
		h := obj.Hostnames[i]
		instance, exists := obj.instances[h]
		if !exists {
			continue
		}
		if e := instance.Kill(); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

// Quit sends a friendly shutdown request to the cluster. You can specify a
// context if you'd like to exit earlier. If you trigger an early exit with the
// context, then this will end up running a `kill -9` so it can return. Remember
// to leave a longer timeout when using a context since this will have to call
// quit on each member individually.
func (obj *Cluster) Quit(ctx context.Context) error {
	var err error
	// do this in reverse for fun
	for i := len(obj.Hostnames) - 1; i >= 0; i-- {
		h := obj.Hostnames[i]
		instance, exists := obj.instances[h]
		if !exists {
			continue
		}
		if e := instance.Quit(ctx); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

// Wait until the first converged state is hit for each member in the cluster.
// Remember to leave a longer timeout when using a context since this will have
// to call wait on each member individually.
func (obj *Cluster) Wait(ctx context.Context) error {
	var err error
	for _, h := range obj.Hostnames {
		instance, exists := obj.instances[h]
		if !exists {
			continue
		}
		// TODO: do we want individual waits?
		//ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		//defer cancel()
		if e := instance.Wait(ctx); e != nil {
			err = multierr.Append(err, e)
		}
	}
	return err
}

// DeployLang deploys some code to the cluster. It arbitrarily picks the first
// host to run the deploy on.
func (obj *Cluster) DeployLang(code string) error {
	if len(obj.Hostnames) == 0 {
		return fmt.Errorf("must have at least one host to deploy")
	}
	h := obj.Hostnames[0]
	instance, exists := obj.instances[h]
	if !exists {
		return fmt.Errorf("instance `%s` not found", h)
	}
	return instance.DeployLang(code)
}

// Instances returns the map of instances attached to this cluster. It is most
// useful after a cluster has started. Before Init, it won't have any entries.
func (obj *Cluster) Instances() map[string]*Instance {
	return obj.instances
}

// Dir returns the dir where the instance can write to. You should only use this
// after Init has been called, or it won't have been created and determined yet.
func (obj *Cluster) Dir() string {
	return obj.dir
}

// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"time"

	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const etcdHostname = "etcd" // a unique hostname to use for a single etcd server

// Cluster represents an mgmt cluster. It uses the instance building blocks to
// run clustered tests.
type Cluster struct {
	// Etcd specifies if we should run a standalone etcd instance instead of
	// using the automatic, built-in etcd clustering.
	Etcd bool

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

	etcdInstance *Instance

	instances map[string]*Instance
}

// Init runs some initialization for this Cluster. It errors if the struct was
// populated in an invalid way, or if it can't initialize correctly.
func (obj *Cluster) Init() error {
	var err error

	// create temporary directory to use during testing
	if obj.dir == "" {
		obj.dir, err = os.MkdirTemp("", "mgmt-integration-cluster-")
		if err != nil {
			return errwrap.Wrapf(err, "can't create temporary directory")
		}
	}

	if obj.Etcd {
		if util.StrInList(etcdHostname, obj.Hostnames) {
			return fmt.Errorf("can't use special `%s` hostname for regular hosts", etcdHostname)
		}

		h := etcdHostname
		instancePrefix := path.Join(obj.dir, h)
		if err := os.MkdirAll(instancePrefix, dirMode); err != nil {
			return errwrap.Wrapf(err, "can't create instance directory")
		}

		obj.etcdInstance = &Instance{
			Etcd:     true,
			Hostname: h,
			Preserve: obj.Preserve,
			Logf: func(format string, v ...interface{}) {
				obj.Logf(fmt.Sprintf("instance <%s>: ", h)+format, v...)
			},
			Debug: obj.Debug,

			dir: instancePrefix,
		}
		if err := obj.etcdInstance.Init(); err != nil {
			return errwrap.Wrapf(err, "can't create etcd instance")
		}
	}

	obj.instances = make(map[string]*Instance)

	for _, hostname := range obj.Hostnames {
		h := hostname
		instancePrefix := path.Join(obj.dir, h)
		if err := os.MkdirAll(instancePrefix, dirMode); err != nil {
			return errwrap.Wrapf(err, "can't create instance directory")
		}

		obj.instances[h] = &Instance{
			EtcdServer: obj.Etcd, // is the 0th instance an etcd?
			Hostname:   h,
			Preserve:   obj.Preserve,
			Logf: func(format string, v ...interface{}) {
				obj.Logf(fmt.Sprintf("instance <%s>: ", h)+format, v...)
			},
			Debug: obj.Debug,

			dir: instancePrefix,
		}
		err = errwrap.Append(err, obj.instances[h].Init())
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
		err = errwrap.Append(err, instance.Close())
	}
	if obj.Etcd {
		if err := obj.etcdInstance.Close(); err != nil {
			return errwrap.Wrapf(err, "can't close etcd instance")
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
	if obj.Etcd {
		// Start etcd standalone via `mgmt etcd` built-in sub command.
		if err := obj.etcdInstance.Run(nil); err != nil {
			return errwrap.Wrapf(err, "trouble running etcd")
		}

		// FIXME: Do we need to wait for etcd to startup?
		// wait for startup before continuing with the next one
		//ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		//defer cancel()
		//if err := obj.etcdInstance.Wait(ctx); err != nil { // wait to get a converged signal
		//	return errwrap.Wrapf(err, "mgmt wait on etcd failed") // timeout expired
		//}
	}

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

		if obj.Etcd {
			seeds = []*Instance{obj.etcdInstance} // use main etcd
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
		err = errwrap.Append(err, instance.Kill())
	}
	if obj.Etcd {
		err = errwrap.Append(err, obj.etcdInstance.Kill())
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
		err = errwrap.Append(err, instance.Quit(ctx))
	}

	isInterruptSignal := func(err error) bool {
		if err == nil {
			return false // not an error ;)
		}
		exitErr, ok := err.(*exec.ExitError) // embeds an os.ProcessState
		if !ok {
			return false // not an ExitError
		}

		pStateSys := exitErr.Sys() // (*os.ProcessState) Sys
		wStatus, ok := pStateSys.(syscall.WaitStatus)
		if !ok {
			return false // not what we're looking for
		}
		if !wStatus.Signaled() {
			return false // not a timeout or cancel (no signal)
		}
		sig := wStatus.Signal()
		//exitStatus := wStatus.ExitStatus() // exitStatus == -1

		if sig != os.Interrupt {
			return false // wrong signal
		}

		return true
	}

	if obj.Etcd {
		// etcd exits non-zero if you ^C it, so ignore it if it's that!
		if err := obj.etcdInstance.Quit(ctx); err != nil && !isInterruptSignal(err) {
			err = errwrap.Append(err, obj.etcdInstance.Quit(ctx))
		}
	}
	return err
}

// Wait until the first converged state is hit for each member in the cluster.
// Remember to leave a longer timeout when using a context since this will have
// to call wait on each member individually.
func (obj *Cluster) Wait(ctx context.Context) error {
	var err error
	// TODO: not implemented
	//if obj.Etcd {
	//	err = errwrap.Append(err, obj.etcdInstance.Wait(ctx))
	//}
	for _, h := range obj.Hostnames {
		instance, exists := obj.instances[h]
		if !exists {
			continue
		}
		// TODO: do we want individual waits?
		//ctx, cancel := context.WithTimeout(context.Background(), longTimeout*time.Second)
		//defer cancel()
		err = errwrap.Append(err, instance.Wait(ctx))
	}
	return err
}

// DeployLang deploys some code to the cluster. It arbitrarily picks the first
// host to run the deploy on unless there is an etcd server running.
func (obj *Cluster) DeployLang(code string) error {
	if obj.Etcd {
		return obj.etcdInstance.DeployLang(code)
	}

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

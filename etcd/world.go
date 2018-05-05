// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package etcd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/etcd/chooser"
	"github.com/purpleidea/mgmt/etcd/client"
	"github.com/purpleidea/mgmt/etcd/client/resources"
	"github.com/purpleidea/mgmt/etcd/client/str"
	"github.com/purpleidea/mgmt/etcd/client/strmap"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/etcd/scheduler"
	"github.com/purpleidea/mgmt/util"
)

// World is an etcd backed implementation of the World interface.
type World struct {
	Hostname       string // uuid for the consumer of these
	Client         interfaces.Client
	MetadataPrefix string    // expected metadata prefix
	StoragePrefix  string    // storage prefix for etcdfs storage
	StandaloneFs   engine.Fs // store an fs here for local usage
	Debug          bool
	Logf           func(format string, v ...interface{})
}

// ResWatch returns a channel which spits out events on possible exported
// resource changes.
func (obj *World) ResWatch(ctx context.Context) (chan error, error) {
	return resources.WatchResources(ctx, obj.Client)
}

// ResExport exports a list of resources under our hostname namespace.
// Subsequent calls replace the previously set collection atomically.
func (obj *World) ResExport(ctx context.Context, resourceList []engine.Res) error {
	return resources.SetResources(ctx, obj.Client, obj.Hostname, resourceList)
}

// ResCollect gets the collection of exported resources which match the filter.
// It does this atomically so that a call always returns a complete collection.
func (obj *World) ResCollect(ctx context.Context, hostnameFilter, kindFilter []string) ([]engine.Res, error) {
	// XXX: should we be restricted to retrieving resources that were
	// exported with a tag that allows or restricts our hostname? We could
	// enforce that here if the underlying API supported it... Add this?
	return resources.GetResources(ctx, obj.Client, hostnameFilter, kindFilter)
}

// IdealClusterSizeWatch returns a stream of errors anytime the cluster-wide
// dynamic cluster size setpoint changes.
func (obj *World) IdealClusterSizeWatch(ctx context.Context) (chan error, error) {
	c := client.NewClientFromSimple(obj.Client, ChooserPath)
	if err := c.Init(); err != nil {
		return nil, err
	}
	util.WgFromCtx(ctx).Add(1)
	go func() {
		util.WgFromCtx(ctx).Done()
		// This must get closed *after* because it will not finish until
		// the Watcher returns, because it contains a wg.Wait() in it...
		defer c.Close() // ignore error
		select {
		case <-ctx.Done():
		}
	}()
	return c.Watcher(ctx, chooser.IdealDynamicSizePath)
}

// IdealClusterSizeGet gets the cluster-wide dynamic cluster size setpoint.
func (obj *World) IdealClusterSizeGet(ctx context.Context) (uint16, error) {
	c := client.NewClientFromSimple(obj.Client, ChooserPath)
	if err := c.Init(); err != nil {
		return 0, err
	}
	defer c.Close()                       // ignore error
	return chooser.DynamicSizeGet(ctx, c) // use client with added namespace
}

// IdealClusterSizeSet sets the cluster-wide dynamic cluster size setpoint.
func (obj *World) IdealClusterSizeSet(ctx context.Context, size uint16) (bool, error) {
	c := client.NewClientFromSimple(obj.Client, ChooserPath)
	if err := c.Init(); err != nil {
		return false, err
	}
	defer c.Close() // ignore error
	return chooser.DynamicSizeSet(ctx, c, size)
}

// StrWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrWatch(ctx context.Context, namespace string) (chan error, error) {
	return str.WatchStr(ctx, obj.Client, namespace)
}

// StrIsNotExist returns whether the error from StrGet is a key missing error.
func (obj *World) StrIsNotExist(err error) bool {
	return err == interfaces.ErrNotExist
}

// StrGet returns the value for the the given namespace.
func (obj *World) StrGet(ctx context.Context, namespace string) (string, error) {
	return str.GetStr(ctx, obj.Client, namespace)
}

// StrSet sets the namespace value to a particular string.
func (obj *World) StrSet(ctx context.Context, namespace, value string) error {
	return str.SetStr(ctx, obj.Client, namespace, &value)
}

// StrDel deletes the value in a particular namespace.
func (obj *World) StrDel(ctx context.Context, namespace string) error {
	return str.SetStr(ctx, obj.Client, namespace, nil)
}

// StrMapWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrMapWatch(ctx context.Context, namespace string) (chan error, error) {
	return strmap.WatchStrMap(ctx, obj.Client, namespace)
}

// StrMapGet returns a map of hostnames to values in the given namespace.
func (obj *World) StrMapGet(ctx context.Context, namespace string) (map[string]string, error) {
	return strmap.GetStrMap(ctx, obj.Client, []string{}, namespace)
}

// StrMapSet sets the namespace value to a particular string under the identity
// of its own hostname.
func (obj *World) StrMapSet(ctx context.Context, namespace, value string) error {
	return strmap.SetStrMap(ctx, obj.Client, obj.Hostname, namespace, &value)
}

// StrMapDel deletes the value in a particular namespace.
func (obj *World) StrMapDel(ctx context.Context, namespace string) error {
	return strmap.SetStrMap(ctx, obj.Client, obj.Hostname, namespace, nil)
}

// Scheduler returns a scheduling result of hosts in a particular namespace.
// XXX: Add a context.Context here
func (obj *World) Scheduler(namespace string, opts ...scheduler.Option) (*scheduler.Result, error) {
	modifiedOpts := []scheduler.Option{}
	for _, o := range opts {
		modifiedOpts = append(modifiedOpts, o) // copy in
	}

	modifiedOpts = append(modifiedOpts, scheduler.Debug(obj.Debug))
	modifiedOpts = append(modifiedOpts, scheduler.Logf(obj.Logf))

	path := fmt.Sprintf(schedulerPathFmt, namespace)
	return scheduler.Schedule(obj.Client.GetClient(), path, obj.Hostname, modifiedOpts...)
}

// Fs returns a distributed file system from a unique URI. For single host
// execution that doesn't span more than a single host, this file system might
// actually be a local or memory backed file system, so actually only
// distributed within the boredom that is a single host cluster.
func (obj *World) Fs(uri string) (engine.Fs, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}

	// we're in standalone mode
	if u.Scheme == "memmapfs" && u.Path == "/" {
		return obj.StandaloneFs, nil
	}

	if u.Scheme != "etcdfs" {
		return nil, fmt.Errorf("unknown scheme: `%s`", u.Scheme)
	}
	if u.Path == "" {
		return nil, fmt.Errorf("empty path: %s", u.Path)
	}
	if !strings.HasPrefix(u.Path, obj.MetadataPrefix) {
		return nil, fmt.Errorf("wrong path prefix: %s", u.Path)
	}

	etcdFs := &etcdfs.Fs{
		Client:     obj.Client, // TODO: do we need to add a namespace?
		Metadata:   u.Path,
		DataPrefix: obj.StoragePrefix,

		Debug: obj.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.Logf("fs: "+format, v...)
		},
	}
	return etcdFs, nil
}

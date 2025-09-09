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
	"github.com/purpleidea/mgmt/etcd/deployer"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/etcd/scheduler"
	"github.com/purpleidea/mgmt/lang/embedded"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

// World is an etcd backed implementation of the World interface.
type World struct {
	// NOTE: Update the etcd/ssh/ World struct if this one changes.

	// Client is the etcd client to use. This should not be specified, one
	// will be created automatically. This exists for legacy reasons and for
	// the SSH etcd world implementation. Maybe it can be removed in the
	// future.
	Client interfaces.Client

	// Seeds are the list of etcd endpoints to connect to.
	Seeds []string

	// NS is the etcd namespace to use.
	NS string

	MetadataPrefix string    // expected metadata prefix
	StoragePrefix  string    // storage prefix for etcdfs storage
	StandaloneFs   engine.Fs // store an fs here for local usage
	GetURI         func() string

	init         *engine.WorldInit
	client       interfaces.Client
	simpleDeploy *deployer.SimpleDeploy

	cleanups []func() error
}

// Connect runs first.
func (obj *World) Connect(ctx context.Context, init *engine.WorldInit) error {
	obj.init = init

	obj.client = obj.Client // legacy default
	if obj.Client == nil {
		c := client.NewClientFromSeedsNamespace(
			obj.Seeds, // endpoints
			obj.NS,
		)
		if err := c.Init(); err != nil {
			return errwrap.Wrapf(err, "client Init failed")
		}
		obj.cleanups = append(obj.cleanups, func() error {
			e := c.Close()
			if obj.init.Debug && e != nil {
				obj.init.Logf("etcd client close error: %+v", e)
			}
			return e
		})
		obj.client = c
	}

	obj.simpleDeploy = &deployer.SimpleDeploy{
		Client: obj.client,
		Debug:  obj.init.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.init.Logf("deploy: "+format, v...)
		},
	}
	if err := obj.simpleDeploy.Init(); err != nil {
		return errwrap.Wrapf(err, "deploy Init failed")
	}
	obj.cleanups = append(obj.cleanups, func() error {
		e := obj.simpleDeploy.Close()
		if obj.init.Debug && e != nil {
			obj.init.Logf("deploy close error: %+v", e)
		}
		return e
	})

	return nil
}

// cleanup performs all the "close" actions either at the very end or as we go.
func (obj *World) cleanup() error {
	var errs error
	for i := len(obj.cleanups) - 1; i >= 0; i-- { // reverse
		f := obj.cleanups[i]
		if err := f(); err != nil {
			errs = errwrap.Append(errs, err)
		}
	}
	obj.cleanups = nil // clean
	return errs
}

// Cleanup runs last.
func (obj *World) Cleanup() error {
	return obj.cleanup()
}

// WatchDeploy returns a channel which spits out events on new deploy activity.
func (obj *World) WatchDeploy(ctx context.Context) (chan error, error) {
	return obj.simpleDeploy.WatchDeploy(ctx)
}

// GetDeploys gets all the available deploys.
func (obj *World) GetDeploys(ctx context.Context) (map[uint64]string, error) {
	return obj.simpleDeploy.GetDeploys(ctx)
}

// GetDeploy returns the deploy with the specified id if it exists.
func (obj *World) GetDeploy(ctx context.Context, id uint64) (string, error) {
	return obj.simpleDeploy.GetDeploy(ctx, id)
}

// GetMaxDeployID returns the maximum deploy id.
func (obj *World) GetMaxDeployID(ctx context.Context) (uint64, error) {
	return obj.simpleDeploy.GetMaxDeployID(ctx)
}

// AddDeploy adds a new deploy.
func (obj *World) AddDeploy(ctx context.Context, id uint64, hash, pHash string, data *string) error {
	return obj.simpleDeploy.AddDeploy(ctx, id, hash, pHash, data)
}

// ResWatch returns a channel which spits out events on possible exported
// resource changes.
func (obj *World) ResWatch(ctx context.Context, kind string) (chan error, error) {
	return resources.WatchResources(ctx, obj.client, obj.init.Hostname, kind)
}

// ResCollect gets the collection of exported resources which match the filters.
// It does this atomically so that a call always returns a complete collection.
func (obj *World) ResCollect(ctx context.Context, filters []*engine.ResFilter) ([]*engine.ResOutput, error) {
	return resources.GetResources(ctx, obj.client, obj.init.Hostname, filters)
}

// ResExport stores a number of resources in the world storage system. The
// individual records should not be updated if they are identical to what is
// already present. (This is to prevent unnecessary events.) If this makes no
// changes, it returns (true, nil). If it makes a change, then it returns
// (false, nil). On any error we return (false, err). It stores the exports
// under our hostname namespace. Subsequent calls do NOT replace the previously
// set collection.
func (obj *World) ResExport(ctx context.Context, resourceExports []*engine.ResExport) (bool, error) {
	return resources.SetResources(ctx, obj.client, obj.init.Hostname, resourceExports)
}

// ResDelete deletes a number of resources in the world storage system. If this
// doesn't delete, it returns (true, nil). If it makes a delete, then it returns
// (false, nil). On any error we return (false, err).
func (obj *World) ResDelete(ctx context.Context, resourceDeletes []*engine.ResDelete) (bool, error) {
	return resources.DelResources(ctx, obj.client, obj.init.Hostname, resourceDeletes)
}

// IdealClusterSizeWatch returns a stream of errors anytime the cluster-wide
// dynamic cluster size setpoint changes.
func (obj *World) IdealClusterSizeWatch(ctx context.Context) (chan error, error) {
	c := client.NewClientFromSimple(obj.client, ChooserPath)
	if err := c.Init(); err != nil {
		return nil, err
	}
	wg := util.WgFromCtx(ctx)
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		if wg != nil {
			defer wg.Done()
		}
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
	c := client.NewClientFromSimple(obj.client, ChooserPath)
	if err := c.Init(); err != nil {
		return 0, err
	}
	defer c.Close()                       // ignore error
	return chooser.DynamicSizeGet(ctx, c) // use client with added namespace
}

// IdealClusterSizeSet sets the cluster-wide dynamic cluster size setpoint.
func (obj *World) IdealClusterSizeSet(ctx context.Context, size uint16) (bool, error) {
	c := client.NewClientFromSimple(obj.client, ChooserPath)
	if err := c.Init(); err != nil {
		return false, err
	}
	defer c.Close() // ignore error
	return chooser.DynamicSizeSet(ctx, c, size)
}

// StrWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrWatch(ctx context.Context, namespace string) (chan error, error) {
	return str.WatchStr(ctx, obj.client, namespace)
}

// StrIsNotExist returns whether the error from StrGet is a key missing error.
func (obj *World) StrIsNotExist(err error) bool {
	return err == interfaces.ErrNotExist
}

// StrGet returns the value for the the given namespace.
func (obj *World) StrGet(ctx context.Context, namespace string) (string, error) {
	return str.GetStr(ctx, obj.client, namespace)
}

// StrSet sets the namespace value to a particular string.
// XXX: This can overwrite another hosts value that was set with StrMapSet. Add
// possible cryptographic signing or special namespacing to prevent such things.
func (obj *World) StrSet(ctx context.Context, namespace, value string) error {
	return str.SetStr(ctx, obj.client, namespace, &value)
}

// StrDel deletes the value in a particular namespace.
func (obj *World) StrDel(ctx context.Context, namespace string) error {
	return str.SetStr(ctx, obj.client, namespace, nil)
}

// StrMapWatch returns a channel which spits out events on possible string
// changes.
func (obj *World) StrMapWatch(ctx context.Context, namespace string) (chan error, error) {
	return strmap.WatchStrMap(ctx, obj.client, namespace)
}

// StrMapGet returns a map of hostnames to values in the given namespace.
func (obj *World) StrMapGet(ctx context.Context, namespace string) (map[string]string, error) {
	return strmap.GetStrMap(ctx, obj.client, []string{}, namespace)
}

// StrMapSet sets the namespace value to a particular string under the identity
// of its own hostname.
func (obj *World) StrMapSet(ctx context.Context, namespace, value string) error {
	return strmap.SetStrMap(ctx, obj.client, obj.init.Hostname, namespace, &value)
}

// StrMapDel deletes the value in a particular namespace.
func (obj *World) StrMapDel(ctx context.Context, namespace string) error {
	return strmap.SetStrMap(ctx, obj.client, obj.init.Hostname, namespace, nil)
}

// Scheduler returns a scheduling result of hosts in a particular namespace.
// XXX: Add a context.Context here
func (obj *World) Scheduler(namespace string, opts ...scheduler.Option) (*scheduler.Result, error) {
	modifiedOpts := []scheduler.Option{}
	for _, o := range opts {
		modifiedOpts = append(modifiedOpts, o) // copy in
	}

	modifiedOpts = append(modifiedOpts, scheduler.Debug(obj.init.Debug))
	modifiedOpts = append(modifiedOpts, scheduler.Logf(obj.init.Logf))

	path := fmt.Sprintf(schedulerPathFmt, namespace)
	return scheduler.Schedule(obj.client, path, obj.init.Hostname, modifiedOpts...)
}

// Scheduled gets the scheduled results without participating.
func (obj *World) Scheduled(ctx context.Context, namespace string) (chan *scheduler.ScheduledResult, error) {
	path := fmt.Sprintf(schedulerPathFmt, namespace)
	return scheduler.Scheduled(ctx, obj.client, path)
}

// URI returns the current FS URI.
// TODO: Can we improve this API or deprecate it entirely?
func (obj *World) URI() string {
	return obj.GetURI()
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

	if u.Scheme == embedded.Scheme {
		path := strings.TrimPrefix(u.Path, "/") // expect a leading slash
		return embedded.Lookup(path)            // does not expect a leading slash
	}

	if u.Scheme != etcdfs.Scheme {
		return nil, fmt.Errorf("unknown scheme: `%s`", u.Scheme)
	}
	if u.Path == "" {
		return nil, fmt.Errorf("empty path: %s", u.Path)
	}
	if !strings.HasPrefix(u.Path, obj.MetadataPrefix) {
		return nil, fmt.Errorf("wrong path prefix: %s", u.Path)
	}

	etcdFs := &etcdfs.Fs{
		Client:     obj.client, // TODO: do we need to add a namespace?
		Metadata:   u.Path,
		DataPrefix: obj.StoragePrefix,

		Debug: obj.init.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.init.Logf("fs: "+format, v...)
		},
	}
	return etcdFs, nil
}

// WatchMembers returns a channel of changing members in the cluster.
func (obj *World) WatchMembers(ctx context.Context) (<-chan *interfaces.MembersResult, error) {
	return obj.client.WatchMembers(ctx)
}

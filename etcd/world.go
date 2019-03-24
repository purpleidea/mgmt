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
	"fmt"
	"net/url"
	"strings"

	"github.com/purpleidea/mgmt/engine"
	etcdfs "github.com/purpleidea/mgmt/etcd/fs"
	"github.com/purpleidea/mgmt/etcd/scheduler"
)

// World is an etcd backed implementation of the World interface.
type World struct {
	Hostname       string // uuid for the consumer of these
	EmbdEtcd       *EmbdEtcd
	MetadataPrefix string    // expected metadata prefix
	StoragePrefix  string    // storage prefix for etcdfs storage
	StandaloneFs   engine.Fs // store an fs here for local usage
	Debug          bool
	Logf           func(format string, v ...interface{})
}

// ResWatch returns a channel which spits out events on possible exported
// resource changes.
func (obj *World) ResWatch() chan error {
	return WatchResources(obj.EmbdEtcd)
}

// ResExport exports a list of resources under our hostname namespace.
// Subsequent calls replace the previously set collection atomically.
func (obj *World) ResExport(resourceList []engine.Res) error {
	return SetResources(obj.EmbdEtcd, obj.Hostname, resourceList)
}

// ResCollect gets the collection of exported resources which match the filter.
// It does this atomically so that a call always returns a complete collection.
func (obj *World) ResCollect(hostnameFilter, kindFilter []string) ([]engine.Res, error) {
	// XXX: should we be restricted to retrieving resources that were
	// exported with a tag that allows or restricts our hostname? We could
	// enforce that here if the underlying API supported it... Add this?
	return GetResources(obj.EmbdEtcd, hostnameFilter, kindFilter)
}

// StrWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrWatch(namespace string) chan error {
	return WatchStr(obj.EmbdEtcd, namespace)
}

// StrIsNotExist returns whether the error from StrGet is a key missing error.
func (obj *World) StrIsNotExist(err error) bool {
	return err == ErrNotExist
}

// StrGet returns the value for the the given namespace.
func (obj *World) StrGet(namespace string) (string, error) {
	return GetStr(obj.EmbdEtcd, namespace)
}

// StrSet sets the namespace value to a particular string.
func (obj *World) StrSet(namespace, value string) error {
	return SetStr(obj.EmbdEtcd, namespace, &value)
}

// StrDel deletes the value in a particular namespace.
func (obj *World) StrDel(namespace string) error {
	return SetStr(obj.EmbdEtcd, namespace, nil)
}

// StrMapWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrMapWatch(namespace string) chan error {
	return WatchStrMap(obj.EmbdEtcd, namespace)
}

// StrMapGet returns a map of hostnames to values in the given namespace.
func (obj *World) StrMapGet(namespace string) (map[string]string, error) {
	return GetStrMap(obj.EmbdEtcd, []string{}, namespace)
}

// StrMapSet sets the namespace value to a particular string under the identity
// of its own hostname.
func (obj *World) StrMapSet(namespace, value string) error {
	return SetStrMap(obj.EmbdEtcd, obj.Hostname, namespace, &value)
}

// StrMapDel deletes the value in a particular namespace.
func (obj *World) StrMapDel(namespace string) error {
	return SetStrMap(obj.EmbdEtcd, obj.Hostname, namespace, nil)
}

// Scheduler returns a scheduling result of hosts in a particular namespace.
func (obj *World) Scheduler(namespace string, opts ...scheduler.Option) (*scheduler.Result, error) {
	modifiedOpts := []scheduler.Option{}
	for _, o := range opts {
		modifiedOpts = append(modifiedOpts, o) // copy in
	}

	modifiedOpts = append(modifiedOpts, scheduler.Debug(obj.Debug))
	modifiedOpts = append(modifiedOpts, scheduler.Logf(obj.Logf))

	return scheduler.Schedule(obj.EmbdEtcd.GetClient(), fmt.Sprintf("%s/scheduler/%s", NS, namespace), obj.Hostname, modifiedOpts...)
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
		Client:     obj.EmbdEtcd.GetClient(),
		Metadata:   u.Path,
		DataPrefix: obj.StoragePrefix,
	}
	return etcdFs, nil
}

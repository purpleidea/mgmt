// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package etcd

import (
	"github.com/purpleidea/mgmt/resources"
)

// World is an etcd backed implementation of the World interface.
type World struct {
	Hostname string // uuid for the consumer of these
	EmbdEtcd *EmbdEtcd
}

// ResExport exports a list of resources under our hostname namespace.
// Subsequent calls replace the previously set collection atomically.
func (obj *World) ResExport(resourceList []resources.Res) error {
	return SetResources(obj.EmbdEtcd, obj.Hostname, resourceList)
}

// ResCollect gets the collection of exported resources which match the filter.
// It does this atomically so that a call always returns a complete collection.
func (obj *World) ResCollect(hostnameFilter, kindFilter []string) ([]resources.Res, error) {
	// XXX: should we be restricted to retrieving resources that were
	// exported with a tag that allows or restricts our hostname? We could
	// enforce that here if the underlying API supported it... Add this?
	return GetResources(obj.EmbdEtcd, hostnameFilter, kindFilter)
}

// SetWatch returns a channel which spits out events on possible string changes.
func (obj *World) StrWatch(namespace string) chan error {
	return WatchStr(obj.EmbdEtcd, namespace)
}

// StrGet returns a map of hostnames to values in the given namespace.
func (obj *World) StrGet(namespace string) (map[string]string, error) {
	return GetStr(obj.EmbdEtcd, []string{}, namespace)
}

// StrSet sets the namespace value to a particular string under the identity of
// its own hostname.
func (obj *World) StrSet(namespace, value string) error {
	return SetStr(obj.EmbdEtcd, obj.Hostname, namespace, &value)
}

// StrDel deletes the value in a particular namespace.
func (obj *World) StrDel(namespace string) error {
	return SetStr(obj.EmbdEtcd, obj.Hostname, namespace, nil)
}

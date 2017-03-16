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
  etcd "github.com/coreos/etcd/clientv3" // "clientv3"
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


// ResWatch returns a channel that outputs a true bool when activity occurs
// TODO: Filter our watch (on the server side if possible) based on the
// collection prefixes and filters that we care about...
func (obj *World) ResWatch() chan bool {
  ch := make(chan bool, 1) // buffer it so we can measure it
  path := fmt.Sprintf("/%s/exported/", NS)
  callback := func(re *RE) error {
    // TODO: is this even needed? it used to happen on conn errors
    log.Printf("Etcd: Watch: Path: %v", path) // event
    if re == nil || re.response.Canceled {
      return fmt.Errorf("watch is empty") // will cause a CtxError+retry
    }
    // we normally need to check if anything changed since the last
    // event, since a set (export) with no changes still causes the
    // watcher to trigger and this would cause an infinite loop. we
    // don't need to do this check anymore because we do the export
    // transactionally, and only if a change is needed. since it is
    // atomic, all the changes arrive together which avoids dupes!!
    if len(ch) == 0 { // send event only if one isn't pending
      // this check avoids multiple events all queueing up and then
      // being released continuously long after the changes stopped
      // do not block!
      ch <- true // event
    }
    return nil
  }
  _, _ = obj.EmbdEtcd.AddWatcher(path, callback, true, false, etcd.WithPrefix()) // no need to check errors
  return ch
}

// ResCollect gets the collection of exported resources which match the filter.
// It does this atomically so that a call always returns a complete collection.
func (obj *World) ResCollect(hostnameFilter, kindFilter []string) ([]resources.Res, error) {
	// XXX: should we be restricted to retrieving resources that were
	// exported with a tag that allows or restricts our hostname? We could
	// enforce that here if the underlying API supported it... Add this?
	return GetResources(obj.EmbdEtcd, hostnameFilter, kindFilter)
}

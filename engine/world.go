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

package engine

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/etcd/scheduler" // XXX: abstract this if possible
)

// WorldInit is some data passed in when starting the World interface.
// TODO: This is a lousy struct name, feel free to change it.
type WorldInit struct {
	// Hostname is the UUID we use to represent ourselves to everyone else.
	Hostname string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// World is an interface to the rest of the different graph state. It allows the
// GAPI to store state and exchange information throughout the cluster. It is
// the interface each machine uses to communicate with the rest of the world.
type World interface { // TODO: is there a better name for this interface?
	// Connect sets things up and is called once before any other methods.
	Connect(context.Context, *WorldInit) error

	// Cleanup does some cleanup and is the last method that is ever called.
	Cleanup() error

	FsWorld

	DeployWorld

	StrWorld

	ResWorld
}

// FsWorld is a world interface for dealing with the core deploy filesystem
// stuff.
type FsWorld interface {
	// URI returns the current FS URI.
	// TODO: Can we improve this API or deprecate it entirely?
	URI() string

	// Fs takes a URI and returns the filesystem that corresponds to that.
	// This is a way to turn a unique string handle into an appropriate
	// filesystem object that we can interact with.
	Fs(uri string) (Fs, error)
}

// DeployWorld is a world interface with all of the deploy functions.
type DeployWorld interface {
	WatchDeploy(context.Context) (chan error, error)

	// TODO: currently unused, but already implemented
	//GetDeploys(ctx context.Context) (map[uint64]string, error)

	GetDeploy(ctx context.Context, id uint64) (string, error)

	GetMaxDeployID(ctx context.Context) (uint64, error)

	// TODO: This could be split out to a sub-interface?
	AddDeploy(ctx context.Context, id uint64, hash, pHash string, data *string) error
}

// StrWorld is a world interface which is useful for reading, writing, and
// watching strings in a shared, distributed database. It is likely that much of
// the functionality is built upon these primitives.
// XXX: We should consider improving this API if possible.
type StrWorld interface {
	StrWatch(ctx context.Context, namespace string) (chan error, error)
	StrIsNotExist(error) bool
	StrGet(ctx context.Context, namespace string) (string, error)
	StrSet(ctx context.Context, namespace, value string) error
	StrDel(ctx context.Context, namespace string) error

	// XXX: add the exchange primitives in here directly?
	StrMapWatch(ctx context.Context, namespace string) (chan error, error)
	StrMapGet(ctx context.Context, namespace string) (map[string]string, error)
	StrMapSet(ctx context.Context, namespace, value string) error
	StrMapDel(ctx context.Context, namespace string) error
}

// ResWorld is a world interface that lets us store, pull and watch resources in
// a distributed database.
// XXX: These API's are likely to change.
// XXX: Add optional TTL's to these API's, maybe use WithTTL(...) type options.
// XXX: Add a WithStar(true) option to add in the * hostname matching.
type ResWorld interface {
	// ResWatch returns a channel which produces a new value once on startup
	// as soon as it is successfully connected, and once for every time it
	// sees that a resource that has been exported for this hostname is
	// added, deleted, or modified. If kind is specified, the watch will
	// attempt to only send events relating to that resource kind. We always
	// intended to only show events for resources which the watching host is
	// allowed to see.
	ResWatch(ctx context.Context, kind string) (chan error, error)

	// ResCollect does a lookup for resource entries that have previously
	// been stored for us. It returns a subset of these based on the input
	// filter. It does not return a Res, since while that would be useful,
	// and logical, it turns out we usually want to transport the Res data
	// onwards through the function graph, and using a native string is what
	// is already supported. (A native res type would just be encoded as a
	// string anyways.) While it might be more "correct" to do the work to
	// decode the string into a Res, the user of this function would just
	// encode it back to a string anyways, and this is not very efficient.
	ResCollect(ctx context.Context, filters []*ResFilter) ([]*ResOutput, error)

	// ResExport stores a number of resources in the world storage system.
	// The individual records should not be updated if they are identical to
	// what is already present. (This is to prevent unnecessary events.) If
	// this makes no changes, it returns (true, nil). If it makes a change,
	// then it returns (false, nil). On any error we return (false, err).
	ResExport(ctx context.Context, resourceExports []*ResExport) (bool, error)

	// ResDelete deletes a number of resources in the world storage system.
	// If this doesn't delete, it returns (true, nil). If it makes a delete,
	// then it returns (false, nil). On any error we return (false, err).
	ResDelete(ctx context.Context, resourceDeletes []*ResDelete) (bool, error)
}

// ResFilter specifies that we want to match an item with this three tuple. If
// any of these are empty, then it means to match an item with any value for
// that field.
// TODO: Future secure implementations must verify that the exported made a
// value available to that hostname. It's not enough for a host to request it.
// We can enforce this with public key encryption eventually.
type ResFilter struct {
	Kind string
	Name string
	Host string // from this host
}

// Match returns nil on a successful match.
func (obj *ResFilter) Match(kind, name, host string) error {
	if obj.Kind != "" && obj.Kind != kind {
		return fmt.Errorf("kind did not match")
	}
	if obj.Name != "" && obj.Name != name {
		return fmt.Errorf("name did not match")
	}
	if obj.Host != "" && obj.Host != host {
		return fmt.Errorf("host did not match")
	}

	return nil // match!
}

// MatchFilters is a simple helper function to avoid duplicating this loop here.
// If any filter matches, it returns nil. Otherwise we error.
func MatchFilters(filters []*ResFilter, kind, name, host string) error {
	// TODO: I'd love to avoid this O(N^2) matching if possible...
	for _, filter := range filters {
		if err := filter.Match(kind, name, host); err == nil {
			return nil
		}
	}

	return fmt.Errorf("not matches found") // did not match
}

// ResOutput represents a record of exported resource data which we have read
// out from the world storage system. The Data field contains an encoded version
// of the resource, and even though decoding it will get you a Kind and Name, we
// still store those values here in duplicate for them to be available before
// decoding.
type ResOutput struct {
	Kind string
	Name string
	Host string // from this host
	Data string // encoded res data
}

// ResExport represents a record of exported resource data which we want to save
// to the world storage system. The Data field contains an encoded version of
// the resource, and even though decoding it will get you a Kind and Name, we
// still store those values here in duplicate for them to be available before
// decoding. If Host is specified, then only the node with that hostname may
// access this resource. If it's empty than it may be collected by anyone. If we
// want to export to only three hosts, then we duplicate this entry three times.
// It's true that this is not an efficient use of storage space, but it maps
// logically to a future data structure where data is encrypted to the public
// key of that specific host where we wouldn't be able to de-duplicate anyways.
type ResExport struct {
	Kind string
	Name string
	Host string // to/for this host
	Data string // encoded res data
}

// ResDelete represents the uniqueness key for stored resources. As a result,
// this triple is a useful map key in various locations.
type ResDelete struct {
	Kind string
	Name string
	Host string // to/for this host
}

// SchedulerWorld is an interface that has to do with distributed scheduling.
// XXX: This should be abstracted to remove the etcd specific types if possible.
type SchedulerWorld interface {
	// Scheduler runs a distributed scheduler.
	Scheduler(namespace string, opts ...scheduler.Option) (*scheduler.Result, error)

	// Scheduled gets the scheduled results without participating.
	Scheduled(ctx context.Context, namespace string) (chan *scheduler.ScheduledResult, error)
}

// EtcdWorld is a world interface that should be implemented if the world
// backend is implementing etcd, and if it supports dynamically resizing things.
// TODO: In theory we could generalize this to support other backends, but lets
// assume it's specific to etcd only for now.
type EtcdWorld interface {
	IdealClusterSizeWatch(context.Context) (chan error, error)
	IdealClusterSizeGet(context.Context) (uint16, error)
	IdealClusterSizeSet(context.Context, uint16) (bool, error)

	// WatchMembers returns a channel of changing members in the cluster.
	WatchMembers(context.Context) (<-chan *interfaces.MembersResult, error)
}

// EmbdEtcdWorld is a world interface that should be implemented if the world
// backend is implementing embedded etcd.
// TODO: In theory we could generalize this to support other backends, but lets
// assume it's specific to etcd only for now.
type EmbdEtcdWorld interface {
}

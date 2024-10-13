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

package engine

import (
	"context"

	"github.com/purpleidea/mgmt/etcd/interfaces"
	"github.com/purpleidea/mgmt/etcd/scheduler"
)

// World is an interface to the rest of the different graph state. It allows the
// GAPI to store state and exchange information throughout the cluster. It is
// the interface each machine uses to communicate with the rest of the world.
type World interface { // TODO: is there a better name for this interface?
	ResWatch(context.Context) (chan error, error)
	ResExport(context.Context, []Res) error
	// FIXME: should this method take a "filter" data struct instead of many args?
	ResCollect(ctx context.Context, hostnameFilter, kindFilter []string) ([]Res, error)

	IdealClusterSizeWatch(context.Context) (chan error, error)
	IdealClusterSizeGet(context.Context) (uint16, error)
	IdealClusterSizeSet(context.Context, uint16) (bool, error)

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

	Scheduler(namespace string, opts ...scheduler.Option) (*scheduler.Result, error)

	// URI returns the current FS URI.
	// TODO: Can we improve this API or deprecate it entirely?
	URI() string

	// Fs takes a URI and returns the filesystem that corresponds to that.
	// This is a way to turn a unique string handle into an appropriate
	// filesystem object that we can interact with.
	Fs(uri string) (Fs, error)

	// WatchMembers returns a channel of changing members in the cluster.
	WatchMembers(context.Context) (<-chan *interfaces.MembersResult, error)
}

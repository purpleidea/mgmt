// Mgmt
// Copyright (C) 2013-2021+ James Shubin and the project contributors
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

package engine

import (
	"context"

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

	Fs(uri string) (Fs, error)
}

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

package chooser

import (
	"context"

	"github.com/purpleidea/mgmt/etcd/interfaces"

	etcdtypes "go.etcd.io/etcd/client/pkg/v3/types"
)

// Data represents the input data that is passed to the chooser.
type Data struct {
	// Hostname is the hostname running this chooser instance. It can be
	// used as a unique key in the cluster.
	Hostname string // ourself

	Debug bool
	Logf  func(format string, v ...interface{})
}

// Chooser represents the interface you must implement if you want to be able to
// control which cluster members are added and removed. Remember that this can
// get run from any peer (server) machine in the cluster, and that this may
// change as different leaders are elected! Do not assume any state will remain
// between invocations. If you want to maintain hysteresis or state, make sure
// to synchronize it in etcd.
type Chooser interface {
	// Validate validates the chooser implementation to ensure the params
	// represent a valid instantiation.
	Validate() error

	// Init initializes the chooser and passes in some useful data and
	// handles.
	Init(*Data) error

	// Connect will be called with a client interfaces.Client that you can
	// use if necessary to store some shared state between instances of this
	// and watch for external changes. Sharing state between members should
	// be avoided if possible, and there is no guarantee that your data
	// won't be deleted in a disaster. There are no backups for this,
	// regenerate anything you might need. Additionally, this may only be
	// used inside the Chooser method, since Connect is only called after
	// Init. This is however very useful for implementing special choosers.
	// Since some operations can run on connect, it gets a context. If you
	// cancel this context, then you might expect that Watch could die too.
	// Both of these should get cancelled if you call Disconnect.
	Connect(context.Context, interfaces.Client) error // we get given a namespaced client

	// Disconnect tells us to cancel our use of the client interface that we
	// got from the Connect method. We must not return until we're done.
	Disconnect() error

	// Watch is called by the engine to allow us to Watch for changes that
	// might cause us to want to re-evaluate our nomination decision. It
	// should error if it cannot startup. Once it is running, it should send
	// a nil error on every event, and an error if things go wrong. When
	// Disconnect is shutdown, then that should cause this to exit. When
	// this sends events, Choose will usually eventually get called in
	// response.
	Watch() (chan error, error)

	// Choose takes the current peer membership state, and the available
	// volunteers, and produces a list of who we should add and who should
	// quit. In general, it's best to only remove one member at a time, in
	// particular because this will get called iteratively on future events,
	// and it can remove subsequent members on the next iteration. One
	// important note: when building a new cluster, we do assume that out of
	// one available volunteer, and no members, that this first volunteer is
	// selected. Make sure that any implementations of this function do this
	// as well, since otherwise the hardcoded initial assumption would be
	// proven wrong here!
	// TODO: we could pass in two lists of hostnames instead of the full
	// URLsMap here, but let's keep it more complicated now in case, and
	// reduce it down later if needed...
	// TODO: should we add a step arg here ?
	Choose(membership, volunteers etcdtypes.URLsMap) (nominees, quitters []string, err error)

	// Close runs some cleanup routines in case there is anything that you'd
	// like to free after we're done.
	Close() error
}

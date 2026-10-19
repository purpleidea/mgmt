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

package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource(remoteDoneKind, func() engine.Res { return &RemoteDoneRes{} })
}

const (
	remoteDoneKind = "remote:done"

	// remoteDoneCleanupTimeout bounds the time we spend deleting our done
	// record in Cleanup. If this fails, the lease still deletes it later.
	remoteDoneCleanupTimeout = 10 * time.Second
)

// RemoteDoneRes is a resource which lets a child, which was started by a remote
// resource, tell that remote resource that we're done executing on this child.
// When this resource runs, it passes a message to the parent, which sees that
// we're done, acknowledges it, and then does whatever the Signal param asked
// for, which by default is to send us back a ^C. The parent's remote resource
// then finishes, either successfully, or with the error in the Error param. The
// parent stores that result, and won't launch the child remote execution again,
// until it's cleared that ack flag with the clear param of the remote resource.
// The parent side can use the is_done function in the res/remote package to see
// whether this flag has been set or not too.
//
// The name of this resource must match the name of the remote resource which
// started us. This only works in a child of a transient remote resource for
// now, and you can only really be executing one of these in your graph at a
// time, since this run was obviously started by a single host.

// Keep in mind, that this resource must not be run if the mgmt run where this
// executes is not actually a remote child! If you're not sure, you can use the
// is_child function in the res/remote package to determine if that's the case.
type RemoteDoneRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	// Signal specifies what the parent does after it has recorded that
	// we're done. It must be one of `none`, `exit`, `soft`, or `hard`. The
	// empty string is the default and means `exit`.
	//
	// The `none` value tells the parent to record the result and to
	// acknowledge it, but to send us nothing, and to wait for us to exit by
	// ourselves. Use this when something that runs after this resource is
	// what should end the process, such as with an edge like:
	// Remote:Done["r0"] -> Exit["exit"]. If the parent sent us a ^C as soon
	// as we said we were done, it would race with the exit resource, and
	// the exit resource might lose, because a ^C stops the engine from
	// scheduling any new work. The same is true of any resource after this
	// one, such as one which powers off the host. Since we exit on our own
	// terms in this mode, the parent reports a non-zero exit status as an
	// error. A dropped connection is not an error, since that's what a host
	// which powers itself off looks like.
	//
	// The `exit` value is the equivalent of the parent sending us a ^C. The
	// engine stops scheduling any new work, it waits for whatever is
	// running to finish, and then we exit.
	//
	// The `soft` value is the equivalent of a second ^C. It's a soft
	// interrupt, which also cancels whatever CheckApply is running at that
	// moment, instead of waiting for it to finish.
	//
	// The `hard` value is the equivalent of a third ^C. It's a hard
	// interrupt, which might leave some of your resources in a partial or
	// unknown state. Only use it if something won't respond to a soft
	// interrupt.
	//
	// With `exit`, `soft`, and `hard`, the parent caused our exit, so it
	// ignores our exit status. Once the parent has acknowledged us, changes
	// to this param have no effect, until the parent clears its record.
	Signal string `lang:"signal" yaml:"signal"`

	// Return, if specified, picks the return value that the parent resource
	// will exit with. A true value tells the parent world that we didn't
	// make any substantive changes, while a false value means that we did
	// do something. If you set Error, then you must either omit this or it
	// must be false.
	// XXX: Not implemented (yet?)
	Return *bool `lang:"return" yaml:"return"`

	// Error, if not empty, tells the parent that we're done, but that we
	// failed. After we've shut down, the parent's remote resource fails
	// with this message instead of succeeding. The parent records this, and
	// it keeps on returning it, without launching us again, until the
	// remote resource is used with the clear param. Once the parent has
	// acknowledged us, changes to this param have no effect, until the
	// parent clears its record.
	Error string `lang:"error" yaml:"error"`

	world  engine.RemoteWorld
	remote *local.Remote // what we know about our parent
}

// signal returns the Signal param with the default resolved.
func (obj *RemoteDoneRes) signal() string {
	if obj.Signal == "" {
		return local.RemoteSignalExit
	}
	return obj.Signal
}

// done returns the done record that we store for our parent.
func (obj *RemoteDoneRes) done() *local.RemoteDone {
	return &local.RemoteDone{
		Signal: obj.signal(),
		Error:  obj.Error,
	}
}

// Default returns some sensible defaults for this resource.
func (obj *RemoteDoneRes) Default() engine.Res {
	return &RemoteDoneRes{}
}

// Validate if the params passed in are valid data.
func (obj *RemoteDoneRes) Validate() error {
	return obj.done().Validate()
}

// Init runs some startup code for this resource.
func (obj *RemoteDoneRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	obj.remote = obj.init.Local.Remote
	if obj.remote == nil {
		return fmt.Errorf("not running as a child of a remote resource")
	}

	// These checks can't go in Validate, since they only look at the params
	// and we don't (yet?) have that data available then. They also can't
	// ever change over the lifetime of this process, so erroring and
	// retrying in CheckApply would be pointless, whilst failing here makes
	// us exit so that the parent sees the error quickly.

	// We don't look at a done for any name which isn't our parent's.
	if obj.Name() != obj.remote.Name {
		return fmt.Errorf("the name must match our parent's remote resource name: %s", obj.remote.Name)
	}
	// TODO: Remove this once the remote resource listens for us in every
	// mode, and not only when it's transient.
	if !obj.remote.Transient {
		return fmt.Errorf("our parent's remote resource is not transient, so it won't listen for us")
	}

	world, ok := obj.init.World.(engine.RemoteWorld)
	if !ok {
		return fmt.Errorf("world backend does not support the RemoteWorld interface")
	}
	obj.world = world

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done. This
// deletes our done record which matters when a graph swap removes us. When the
// whole process exits, the lease which the record is bound to also deletes it.
// It is a latch which transfers to the parent caller, and it's up to that res
// to unlatch it for future progress.
func (obj *RemoteDoneRes) Cleanup() error {
	if obj.world == nil {
		return nil // Init didn't get that far
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteDoneCleanupTimeout)
	defer cancel()
	return obj.world.RemoteDoneDel(ctx, obj.remote.Parent, obj.Name())
}

// watch starts watching the ack record of our parent, and our own done record,
// and returns the event channels of each, in that order.
func (obj *RemoteDoneRes) watch(ctx context.Context) (chan error, chan error, error) {
	ackChan, err := obj.world.RemoteAckWatch(ctx, obj.remote.Parent, obj.Name())
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "can't watch the ack record")
	}
	doneChan, err := obj.world.RemoteDoneWatch(ctx, obj.init.Hostname, obj.remote.Parent, obj.Name())
	if err != nil {
		return nil, nil, errwrap.Wrapf(err, "can't watch our done record")
	}
	return ackChan, doneChan, nil
}

// Watch is the primary listener for this resource and it outputs events. It
// watches the ack record of our parent, and our own done record, which could
// vanish if the lease it's bound to was lost.
func (obj *RemoteDoneRes) Watch(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the watches when we return

	ackChan, doneChan, err := obj.watch(ctx)
	if err != nil {
		return err
	}

	for {
		select {
		case err, ok := <-ackChan:
			if err := watchErr(ctx, "ack", err, ok); err != nil {
				return err
			}

		case err, ok := <-doneChan:
			if err := watchErr(ctx, "done", err, ok); err != nil {
				return err
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return ctx.Err()
		}

		// The first of these comes from the startup events.
		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply stores our done record, and then waits for our parent to
// acknowledge it. The state is okay when both of those records exist.
func (obj *RemoteDoneRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	done := obj.done()
	value, err := done.Encode()
	if err != nil {
		return false, err
	}

	x, err := obj.world.RemoteDoneGet(ctx, obj.init.Hostname, obj.remote.Parent, obj.Name())
	if err != nil {
		return false, err
	}
	ack, err := obj.world.RemoteAckGet(ctx, obj.remote.Parent, obj.Name())
	if err != nil {
		return false, err
	}
	if x != nil && *x == value && ack != nil {
		return true, nil
	}
	if !apply {
		return false, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // stops the watches when we return

	// Start watching before we read anything, so that we can't miss any
	// change between that read and the watch.
	ackChan, doneChan, err := obj.watch(ctx)
	if err != nil {
		return false, err
	}

	obj.init.Logf("sending done to %s: %s", obj.remote.Parent, done)
	for {
		// This writes nothing if our record is already there with our
		// current lease. Otherwise, eg: if the lease was lost and our
		// record vanished, this creates it again with a new lease.
		if err := obj.world.RemoteDoneSet(ctx, obj.remote.Parent, obj.Name(), value); err != nil {
			return false, err
		}

		ack, err := obj.world.RemoteAckGet(ctx, obj.remote.Parent, obj.Name())
		if err != nil {
			return false, err
		}
		if ack != nil {
			obj.init.Logf("got an ack from %s", obj.remote.Parent)
			return false, nil
		}

		select {
		case err, ok := <-ackChan:
			if err := watchErr(ctx, "ack", err, ok); err != nil {
				return false, err
			}

		case err, ok := <-doneChan:
			if err := watchErr(ctx, "done", err, ok); err != nil {
				return false, err
			}

		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *RemoteDoneRes) Cmp(r engine.Res) error {
	// we can only compare RemoteDoneRes to others of the same resource kind
	res, ok := r.(*RemoteDoneRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.signal() != res.signal() {
		return fmt.Errorf("the Signal param differs")
	}
	if obj.Error != res.Error {
		return fmt.Errorf("the Error param differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *RemoteDoneRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes RemoteDoneRes // indirection to avoid infinite recursion

	def := obj.Default()            // get the default
	res, ok := def.(*RemoteDoneRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to RemoteDoneRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = RemoteDoneRes(raw) // restore from indirection with type conversion!
	return nil
}

// watchErr turns the result of a read from a watch channel into an error. A
// watch only closes without sending an error first if its ctx was cancelled.
func watchErr(ctx context.Context, what string, err error, ok bool) error {
	if !ok {
		if e := ctx.Err(); e != nil {
			return e
		}
		return fmt.Errorf("the %s watch closed", what) // should not happen
	}
	return errwrap.Wrapf(err, "the %s watch failed", what)
}

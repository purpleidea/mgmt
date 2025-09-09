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

package coreworld

import (
	"context"
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/etcd/scheduler" // XXX: abstract this if possible
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ScheduleFuncName is the name this function is registered as.
	ScheduleFuncName = "schedule"

	// arg names...
	scheduleArgNameNamespace = "namespace"
)

func init() {
	funcs.ModuleRegister(ModuleName, ScheduleFuncName, func() interfaces.Func { return &ScheduleFunc{} })
}

// ScheduleFunc is special function which determines where code should run in
// the cluster.
type ScheduleFunc struct {
	init  *interfaces.Init
	world engine.SchedulerWorld

	input     chan string // stream of inputs
	namespace *string     // the active namespace

	mutex *sync.Mutex // guards value
	value []string    // list of hosts
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ScheduleFunc) String() string {
	return ScheduleFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ScheduleFunc) ArgGen(index int) (string, error) {
	seq := []string{scheduleArgNameNamespace}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *ScheduleFunc) sig() *types.Type {
	sig := types.NewType(fmt.Sprintf("func(%s str) []str", scheduleArgNameNamespace)) // simplest form
	return sig
}

// Validate tells us if the input struct takes a valid form.
func (obj *ScheduleFunc) Validate() error {
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ScheduleFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		Fast: false,
		Spec: false,
		// output is list of hostnames chosen
		Sig: obj.sig(), // func kind
		Err: obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ScheduleFunc) Init(init *interfaces.Init) error {
	obj.init = init
	world, ok := obj.init.World.(engine.SchedulerWorld)
	if !ok {
		return fmt.Errorf("world backend does not support the SchedulerWorld interface")
	}
	obj.world = world

	obj.input = make(chan string)

	obj.mutex = &sync.Mutex{}
	obj.value = []string{} // empty

	//obj.init.Debug = true // use this for local debugging
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ScheduleFunc) Stream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watch when exiting

	watchChan := make(chan *scheduler.ScheduledResult) // XXX: sender should close this, but did I implement that part yet???

	for {
		select {
		// TODO: should this first chan be run as a priority channel to
		// avoid some sort of glitch? is that even possible? can our
		// hostname check with reality (below) fix that?
		case namespace, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			// TODO: support changing the namespace over time...
			if obj.namespace == nil {
				obj.namespace = &namespace // store it
				var err error
				watchChan, err = obj.world.Scheduled(ctx, namespace) // watch for var changes
				if err != nil {
					return err
				}
				continue // we get values on the watch chan, not here!
			}

			if *obj.namespace == namespace {
				continue // skip duplicates
			}

			// *obj.namespace != namespace
			return fmt.Errorf("can't change namespace, previously: `%s`", *obj.namespace)

		case scheduledResult, ok := <-watchChan:
			if !ok { // closed
				// XXX: maybe etcd reconnected? (fix etcd implementation)

				// XXX: if we close, perhaps the engine is
				// switching etcd hosts and we should retry?
				// maybe instead we should get an "etcd
				// reconnect" signal, and the lang will restart?
				return nil
			}
			if scheduledResult == nil {
				return fmt.Errorf("unexpected nil result")
			}
			if err := scheduledResult.Err; err != nil {
				return errwrap.Wrapf(err, "scheduler result error")
			}

			if obj.init.Debug {
				obj.init.Logf("got hosts: %+v", scheduledResult.Hosts)
			}
			obj.mutex.Lock()
			obj.value = scheduledResult.Hosts // store it
			obj.mutex.Unlock()

			if err := obj.init.Event(ctx); err != nil { // send event
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ScheduleFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	namespace := args[0].Str()

	if namespace == "" {
		return nil, fmt.Errorf("can't use an empty namespace")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	if obj.init.Debug {
		obj.init.Logf("namespace: %s", namespace)
	}

	// Tell the Stream what we're watching now... This doesn't block because
	// Stream should always be ready to consume unless it's closing down...
	// If it dies, then a ctx closure should come soon.
	select {
	case obj.input <- namespace:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	obj.mutex.Lock()   // TODO: could be a read lock
	value := obj.value // initially we might get an empty list
	obj.mutex.Unlock()

	var result types.Value
	l := types.NewList(obj.Info().Sig.Out)
	for _, val := range value {
		if err := l.Add(&types.StrValue{V: val}); err != nil {
			return nil, errwrap.Wrapf(err, "list could not add val: `%s`", val)
		}
	}
	result = l // set list as result

	if obj.init.Debug {
		obj.init.Logf("result: %+v", result)
	}

	return result, nil
}

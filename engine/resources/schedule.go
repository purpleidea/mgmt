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
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/traits"
	"github.com/purpleidea/mgmt/etcd/scheduler" // XXX: abstract this if possible
	"github.com/purpleidea/mgmt/util/errwrap"
)

func init() {
	engine.RegisterResource("schedule", func() engine.Res { return &ScheduleRes{} })
}

// ScheduleRes is a resource which starts up a "distributed scheduler". All
// nodes of the same namespace will be part of the same scheduling pool. The
// scheduling result can be determined by using the "schedule" function. If the
// options specified are different among peers in the same namespace, then it is
// undefined which options if any will get chosen.
type ScheduleRes struct {
	traits.Base // add the base methods without re-implementation

	init *engine.Init

	world engine.SchedulerWorld

	// Namespace represents the namespace key to use. If it is not
	// specified, the Name value is used instead.
	Namespace string `lang:"namespace" yaml:"namespace"`

	// Strategy is the scheduling strategy to use. If this value is nil or,
	// undefined, then a default will be chosen automatically.
	Strategy *string `lang:"strategy" yaml:"strategy"`

	// Max is the max number of hosts to elect. If this is unspecified, then
	// a default of 1 is used.
	Max *int `lang:"max" yaml:"max"`

	// Reuse specifies that we reuse the client lease on reconnect. If reuse
	// is false, then on host disconnect, that hosts entry will immediately
	// expire, and the scheduler will react instantly and remove that host
	// entry from the list. If this is true, or if the host closes without a
	// clean shutdown, it will take the TTL number of seconds to remove the
	// entry.
	Reuse *bool `lang:"reuse" yaml:"reuse"`

	// TTL is the time to live for added scheduling "votes". If this value
	// is nil or, undefined, then a default value is used. See the `Reuse`
	// entry for more information.
	TTL *int `lang:"ttl" yaml:"ttl"`

	// Withdraw specifies whether we should try and remove the host from the
	// scheduling pool. It is incompatible with the other "add to" pool
	// options.
	Withdraw bool `lang:"withdraw" yaml:"withdraw"`

	// once is the startup signal for the scheduler
	once chan struct{}
}

// getNamespace returns the namespace key to be used for this resource. If the
// Namespace field is specified, it will use that, otherwise it uses the Name.
func (obj *ScheduleRes) getNamespace() string {
	if obj.Namespace != "" {
		return obj.Namespace
	}
	return obj.Name()
}

func (obj *ScheduleRes) getOpts() []scheduler.Option {

	schedulerOpts := []scheduler.Option{}
	// don't add bad or zero-value options

	defaultStrategy := true
	if obj.Strategy != nil && *obj.Strategy != "" {
		strategy := *obj.Strategy
		if obj.init.Debug {
			obj.init.Logf("opts: strategy: %s", strategy)
		}
		defaultStrategy = false
		schedulerOpts = append(schedulerOpts, scheduler.StrategyKind(strategy))
	}
	if defaultStrategy { // we always need to add one!
		schedulerOpts = append(schedulerOpts, scheduler.StrategyKind(scheduler.DefaultStrategy))
	}

	if obj.Max != nil && *obj.Max > 0 {
		max := *obj.Max
		// TODO: check for overflow
		if obj.init.Debug {
			obj.init.Logf("opts: max: %d", max)
		}
		schedulerOpts = append(schedulerOpts, scheduler.MaxCount(max))
	}

	if obj.Reuse != nil {
		reuse := *obj.Reuse
		if obj.init.Debug {
			obj.init.Logf("opts: reuse: %t", reuse)
		}
		schedulerOpts = append(schedulerOpts, scheduler.ReuseLease(reuse))
	}

	if obj.TTL != nil && *obj.TTL > 0 {
		ttl := *obj.TTL
		// TODO: check for overflow
		if obj.init.Debug {
			obj.init.Logf("opts: ttl: %d", ttl)
		}
		schedulerOpts = append(schedulerOpts, scheduler.SessionTTL(ttl))
	}

	if obj.Withdraw {
		if obj.init.Debug {
			obj.init.Logf("opts: withdraw: %t", obj.Withdraw)
		}
		schedulerOpts = append(schedulerOpts, scheduler.Withdraw(obj.Withdraw))
	}

	return schedulerOpts
}

// Default returns some sensible defaults for this resource.
func (obj *ScheduleRes) Default() engine.Res {
	return &ScheduleRes{}
}

// Validate if the params passed in are valid data.
func (obj *ScheduleRes) Validate() error {
	if obj.getNamespace() == "" {
		return fmt.Errorf("the Namespace must not be empty")
	}

	if obj.Withdraw {
		if obj.Strategy != nil {
			return fmt.Errorf("can't combine Withdraw with Strategy")
		}
		if obj.Max != nil {
			return fmt.Errorf("can't combine Withdraw with Max")
		}
		if obj.Reuse != nil {
			return fmt.Errorf("can't combine Withdraw with Reuse")
		}
		if obj.TTL != nil {
			return fmt.Errorf("can't combine Withdraw with TTL")
		}
	}

	return nil
}

// Init initializes the resource.
func (obj *ScheduleRes) Init(init *engine.Init) error {
	obj.init = init // save for later

	world, ok := obj.init.World.(engine.SchedulerWorld)
	if !ok {
		return fmt.Errorf("world backend does not support the SchedulerWorld interface")
	}
	obj.world = world

	obj.once = make(chan struct{}, 1) // buffered!

	return nil
}

// Cleanup is run by the engine to clean up after the resource is done.
func (obj *ScheduleRes) Cleanup() error {
	return nil
}

// Watch is the primary listener for this resource and it outputs events.
func (obj *ScheduleRes) Watch(ctx context.Context) error {
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := obj.init.Running(ctx); err != nil { return err } // when started, notify engine that we're running

	select {
	case <-obj.once:
		// pass
	case <-ctx.Done():
		return ctx.Err()
	}

	if obj.init.Debug {
		obj.init.Logf("starting scheduler...")
	}

	sched, err := obj.world.Scheduler(obj.getNamespace(), obj.getOpts()...)
	if err != nil {
		return errwrap.Wrapf(err, "can't create scheduler")
	}

	watchChan := make(chan *scheduler.ScheduledResult)

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer sched.Shutdown()
		select {
		case <-ctx.Done():
			return
		}
	}()

	// process the stream of scheduling output...
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(watchChan)
		for {
			hosts, err := sched.Next(ctx)
			select {
			case watchChan <- &scheduler.ScheduledResult{
				Hosts: hosts,
				Err:   err,
			}:

			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case result, ok := <-watchChan:
			if !ok { // channel shutdown
				return nil
			}
			if result == nil {
				return fmt.Errorf("unexpected nil result")
			}
			if err := result.Err; err != nil {
				if err == scheduler.ErrEndOfResults {
					//return nil // TODO: we should probably fix the reconnect issue and use this here
					return fmt.Errorf("scheduler shutdown, reconnect bug?") // XXX: fix etcd reconnects
				}
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.getNamespace())
			}

			if obj.init.Debug {
				obj.init.Logf("event!")
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if err := obj.init.Event(ctx); err != nil { return err } // notify engine of an event (this can block)
	}
}

// CheckApply method for resource.
func (obj *ScheduleRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// For maximum correctness, don't start scheduling myself until this
	// CheckApply runs at least once. Effectively this unblocks Watch() once
	// it has run. If we didn't do this, then illogical graphs could happen
	// where we have an edge like Foo["whatever"] -> Schedule["bar"] and if
	// Foo failed, we'd still be scheduling, which is not what we want.

	select {
	case obj.once <- struct{}{}:
	default: // if buffer is full
	}

	// FIXME: If we wanted to be really fancy, we could wait until the write
	// to the scheduler (etcd) finished before we returned true.
	return true, nil
}

// Cmp compares two resources and returns an error if they are not equivalent.
func (obj *ScheduleRes) Cmp(r engine.Res) error {
	// we can only compare ScheduleRes to others of the same resource kind
	res, ok := r.(*ScheduleRes)
	if !ok {
		return fmt.Errorf("not a %s", obj.Kind())
	}

	if obj.getNamespace() != res.getNamespace() {
		return fmt.Errorf("the Namespace differs")
	}

	if (obj.Strategy == nil) != (res.Strategy == nil) { // xor
		return fmt.Errorf("the Strategy differs")
	}
	if obj.Strategy != nil && res.Strategy != nil {
		if *obj.Strategy != *res.Strategy { // compare the values
			return fmt.Errorf("the contents of Strategy differs")
		}
	}

	if (obj.Max == nil) != (res.Max == nil) { // xor
		return fmt.Errorf("the Max differs")
	}
	if obj.Max != nil && res.Max != nil {
		if *obj.Max != *res.Max { // compare the values
			return fmt.Errorf("the contents of Max differs")
		}
	}

	if (obj.Reuse == nil) != (res.Reuse == nil) { // xor
		return fmt.Errorf("the Reuse differs")
	}
	if obj.Reuse != nil && res.Reuse != nil {
		if *obj.Reuse != *res.Reuse { // compare the values
			return fmt.Errorf("the contents of Reuse differs")
		}
	}

	if (obj.TTL == nil) != (res.TTL == nil) { // xor
		return fmt.Errorf("the TTL differs")
	}
	if obj.TTL != nil && res.TTL != nil {
		if *obj.TTL != *res.TTL { // compare the values
			return fmt.Errorf("the contents of TTL differs")
		}
	}

	if obj.Withdraw != res.Withdraw {
		return fmt.Errorf("the Withdraw differs")
	}

	return nil
}

// UnmarshalYAML is the custom unmarshal handler for this struct. It is
// primarily useful for setting the defaults.
func (obj *ScheduleRes) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type rawRes ScheduleRes // indirection to avoid infinite recursion

	def := obj.Default()          // get the default
	res, ok := def.(*ScheduleRes) // put in the right format
	if !ok {
		return fmt.Errorf("could not convert to ScheduleRes")
	}
	raw := rawRes(*res) // convert; the defaults go here

	if err := unmarshal(&raw); err != nil {
		return err
	}

	*obj = ScheduleRes(raw) // restore from indirection with type conversion!
	return nil
}

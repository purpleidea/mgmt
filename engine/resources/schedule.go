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
	"github.com/purpleidea/mgmt/scheduler"
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
	// specified, the Name value is used instead. You must *not* run two of
	// these resources on the same host at the same time with the same
	// namespace value. This field exists as a luxury, but it's safer to use
	// the name field instead which will guarantee namespace uniqueness. If
	// you break this rule, then the behaviour is undefined and you may even
	// cause concurrency races, deadlocks, or panics.
	Namespace string `lang:"namespace" yaml:"namespace"`

	// Strategy is the scheduling strategy to use. If this value is nil or,
	// undefined, then a default will be chosen automatically.
	Strategy *string `lang:"strategy" yaml:"strategy"`

	// Max is the max number of hosts to elect. If this is unspecified, then
	// a default of 1 is used.
	Max *int `lang:"max" yaml:"max"`

	// Persist specifies that we should not remove the scheduled value on
	// disappear or disconnect. In that situation it will persist
	// indefinitely if the TTL is 0, otherwise it should expire when the TTL
	// does. If this is false, then when the *host* disconnects by clean
	// shutdown, then the key will be removed. If the host closes without a
	// clean shutdown, it will take the TTL number of seconds to remove the
	// entry. This differs from the resource disappearing during a graph
	// swap. To remove an entry in that case, use with the `Withdraw` param
	// or the reversal mechanism.
	Persist bool `lang:"persist" yaml:"persist"`

	// TTL is the time to live for added scheduling "votes". If this value
	// is nil or, undefined, then a default value is used. See the `Persist`
	// entry for more information.
	TTL *int `lang:"ttl" yaml:"ttl"`

	// Withdraw specifies whether we should try and remove the host from the
	// scheduling pool. It is incompatible with the other "add to" pool
	// options.
	Withdraw bool `lang:"withdraw" yaml:"withdraw"`

	// XXX: Have some common data fields (like cpu/mem/etc) which we pass
	// directly into etcd from each schedule resource and then into the
	// scheduler (or maybe one day directly into the scheduler from each
	// host) which can be used to make scheduling decisions. This is better
	// than going through the function graph with a data field, because we
	// wouldn't need to constantly go through graph swaps to use live data!
	// Some field could exist here to turn this on/off eg: cpu_data => true.

	// XXX: Pass some data into the scheduler. This could embed querystring
	// style data that is used to make scheduling decisions and so on.
	//Data string `lang:"data" yaml:"data"`
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

	if obj.init.Debug {
		obj.init.Logf("opts: persist: %t", obj.Persist)
	}
	schedulerOpts = append(schedulerOpts, scheduler.Persist(obj.Persist))

	if obj.TTL != nil && *obj.TTL > 0 {
		ttl := *obj.TTL
		// TODO: check for overflow
		if obj.init.Debug {
			obj.init.Logf("opts: ttl: %d", ttl)
		}
		schedulerOpts = append(schedulerOpts, scheduler.SessionTTL(ttl))
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

	if s := obj.Strategy; s != nil {
		if _, err := scheduler.Lookup(*s); err != nil {
			return err
		}
	}

	if ttl := obj.TTL; obj.Persist && ttl != nil && *ttl > 0 {
		return fmt.Errorf("can't combine Persist with TTL")
	}

	if obj.Withdraw {
		if obj.Strategy != nil {
			return fmt.Errorf("can't combine Withdraw with Strategy")
		}
		if obj.Max != nil {
			return fmt.Errorf("can't combine Withdraw with Max")
		}
		if obj.Persist {
			return fmt.Errorf("can't combine Withdraw with Persist")
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

	if obj.init.Debug {
		obj.init.Logf("starting scheduler...")
	}

	ch, err := obj.world.Scheduled(ctx, obj.getNamespace())
	if err != nil {
		return errwrap.Wrapf(err, "can't create scheduler")
	}
	// Orphan determines whether we just abandon the session or whether we
	// run an explicit Close() on it. We never close if we Persist or if we
	// have a pending TTL, which means we let it run down automatically.
	orphan := obj.Persist || (obj.TTL != nil && *obj.TTL > 0)
	defer obj.world.SchedulerCleanup(obj.getNamespace(), orphan)

	if err := obj.init.Event(ctx); err != nil {
		return err
	}

	for {
		select {
		case result, ok := <-ch:
			if !ok { // channel shutdown
				return nil
			}
			if result == nil {
				return fmt.Errorf("unexpected nil result")
			}
			if err := result.Err; err != nil {
				if err == scheduler.ErrSchedulerShutdown {
					return fmt.Errorf("scheduler shutdown, reconnect bug?") // XXX: fix etcd reconnects?
				}
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.getNamespace())
			}

			if obj.init.Debug {
				obj.init.Logf("event!")
			}

		case <-ctx.Done(): // closed by the engine to signal shutdown
			return nil
		}

		if err := obj.init.Event(ctx); err != nil {
			return err
		}
	}
}

// CheckApply method for resource.
func (obj *ScheduleRes) CheckApply(ctx context.Context, apply bool) (bool, error) {
	// For maximum correctness, don't start scheduling myself until this
	// runs at least once. If we didn't do this, then illogical graphs could
	// happen where we have an edge like: Foo["whatever"] -> Schedule["bar"]
	// and if Foo fails, we'd still be scheduling which is not what we want.

	if obj.Withdraw {
		// this respects the "apply" flag
		b, err := obj.world.SchedulerWithdraw(ctx, obj.getNamespace(), apply)
		if !b && apply { // it made a change
			obj.init.Logf("withdrawn")
		}
		return b, err
	}

	b, err := obj.world.SchedulerAdd(ctx, obj.getNamespace(), apply, obj.getOpts()...)
	if !b && apply { // it made a change
		obj.init.Logf("scheduled")
	}
	return b, err
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

	if obj.Persist != res.Persist {
		return fmt.Errorf("the Persist differs")
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

// Background is a worker function which is run once per resource kind as long
// as there is at least one of that kind running in the active resource graph.
// The worker function is the generated (returned) function that is used here.
func (obj *ScheduleRes) Background(handle *engine.BackgroundHandle) engine.BackgroundFunc {
	return func(ctx context.Context, ready chan<- struct{}) error {
		world, ok := handle.World.(engine.SchedulerWorld)
		if !ok {
			return fmt.Errorf("world backend does not support the SchedulerWorld interface")
		}

		// All this for a "running/stopped" message?
		//if handle.Debug {
		//	defer handle.Logf("stopped!")
		//	wg := &sync.WaitGroup{}
		//	defer wg.Wait()
		//	wg.Add(1)
		//	go func() {
		//		defer wg.Done()
		//		select {
		//		case <-ready: // XXX: nope!
		//			handle.Logf("running...")
		//		case <-ctx.Done():
		//		}
		//	}()
		//}

		// This sends the ready signal itself...
		return world.Scheduler(ctx, ready)
	}
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

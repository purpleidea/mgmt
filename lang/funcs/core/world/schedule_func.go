// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

// test with:
// time ./mgmt run --hostname h1 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
// time ./mgmt run --hostname h2 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2381 --server-urls http://127.0.0.1:2382 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
// time ./mgmt run --hostname h3 --seeds http://127.0.0.1:2379 --client-urls http://127.0.0.1:2383 --server-urls http://127.0.0.1:2384 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
// kill h2 (should see h1 and h3 pick [h1, h3] instead)
// restart h2 (should see [h1, h3] as before)
// kill h3 (should see h1 and h2 pick [h1, h2] instead)
// restart h3 (should see [h1, h2] as before)
// kill h3
// kill h2
// kill h1... all done!

package coreworld

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/etcd/scheduler" // TODO: is it okay to import this without abstraction?
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DefaultStrategy is the strategy to use if none has been specified.
	DefaultStrategy = "rr"
)

func init() {
	funcs.ModuleRegister(moduleName, "schedule", func() interfaces.Func { return &SchedulePolyFunc{} })
}

// SchedulePolyFunc is special function which determines where code should run
// in the cluster.
type SchedulePolyFunc struct {
	Type *types.Type // this is the type of value stored in our list

	init *interfaces.Init

	namespace string
	scheduler *scheduler.Result

	last   types.Value
	result types.Value // last calculated output

	watchChan chan *schedulerResult
	closeChan chan struct{}
}

// validOpts returns the available mapping of valid opts fields to types.
func (obj *SchedulePolyFunc) validOpts() map[string]*types.Type {
	return map[string]*types.Type{
		"strategy": types.TypeStr,
		"max":      types.TypeInt,
		"reuse":    types.TypeBool,
		"ttl":      types.TypeInt,
	}
}

// Polymorphisms returns the list of possible function signatures available for
// this static polymorphic function. It relies on type and value hints to limit
// the number of returned possibilities.
func (obj *SchedulePolyFunc) Polymorphisms(partialType *types.Type, partialValues []types.Value) ([]*types.Type, error) {
	// TODO: technically, we could generate all permutations of the struct!
	//variant := []*types.Type{}
	//t0 := types.NewType("func(namespace str) []str")
	//variant = append(variant, t0)
	//validOpts := obj.validOpts()
	//for ? := ? range { // generate all permutations of the struct...
	//	t := types.NewType(fmt.Sprintf("func(namespace str, opts %s) []str", ?))
	//	variant = append(variant, t)
	//}
	//if partialType == nil {
	//	return variant, nil
	//}

	if partialType == nil {
		return nil, fmt.Errorf("zero type information given")
	}

	var typ *types.Type

	if tOut := partialType.Out; tOut != nil {
		if err := tOut.Cmp(types.NewType("[]str")); err != nil {
			return nil, errwrap.Wrapf(err, "return type must be a list of strings")
		}
	}

	ord := partialType.Ord
	if partialType.Map != nil {
		if len(ord) == 0 {
			return nil, fmt.Errorf("must have at least one arg in schedule func")
		}

		if tNamespace, exists := partialType.Map[ord[0]]; exists && tNamespace != nil {
			if err := tNamespace.Cmp(types.TypeStr); err != nil {
				return nil, errwrap.Wrapf(err, "first arg must be an str")
			}
		}
		if len(ord) == 1 {
			return []*types.Type{types.NewType("func(namespace str) []str")}, nil // done!
		}

		if len(ord) != 2 {
			return nil, fmt.Errorf("must have either one or two args in schedule func")
		}

		if tOpts, exists := partialType.Map[ord[1]]; exists {
			if tOpts == nil { // usually a `struct{}`
				typFunc := types.NewType("func(namespace str, opts variant) []str")
				return []*types.Type{typFunc}, nil // solved!
			}

			if tOpts.Kind != types.KindStruct {
				return nil, fmt.Errorf("second arg must be of kind struct")
			}

			validOpts := obj.validOpts()
			for _, name := range tOpts.Ord {
				t := tOpts.Map[name]
				value, exists := validOpts[name]
				if !exists {
					return nil, fmt.Errorf("unexpected opts field: `%s`", name)
				}

				if err := t.Cmp(value); err != nil {
					return nil, errwrap.Wrapf(err, "expected different type for opts field: `%s`", name)
				}
			}

			typ = tOpts // solved
		}
	}

	if typ == nil {
		return nil, fmt.Errorf("not enough type information")
	}

	typFunc := types.NewType(fmt.Sprintf("func(namespace str, opts %s) []str", typ.String()))

	// TODO: type check that the partialValues are compatible

	return []*types.Type{typFunc}, nil // solved!
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *SchedulePolyFunc) Build(typ *types.Type) error {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 1 && len(typ.Ord) != 2 {
		return fmt.Errorf("the schedule function needs either one or two args")
	}
	if typ.Out == nil {
		return fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return fmt.Errorf("invalid input type")
	}

	if err := typ.Out.Cmp(types.NewType("[]str")); err != nil {
		return errwrap.Wrapf(err, "return type must be a list of strings")
	}

	tNamespace, exists := typ.Map[typ.Ord[0]]
	if !exists || tNamespace == nil {
		return fmt.Errorf("first arg must be specified")
	}

	if len(typ.Ord) == 1 {
		obj.Type = nil
		return nil // done early, 2nd arg is absent!
	}
	tOpts, exists := typ.Map[typ.Ord[1]]
	if !exists || tOpts == nil {
		return fmt.Errorf("second argument was missing")
	}

	if tOpts.Kind != types.KindStruct {
		return fmt.Errorf("second argument must be of kind struct")
	}

	validOpts := obj.validOpts()
	for _, name := range tOpts.Ord {
		t := tOpts.Map[name]
		value, exists := validOpts[name]
		if !exists {
			return fmt.Errorf("unexpected opts field: `%s`", name)
		}

		if err := t.Cmp(value); err != nil {
			return errwrap.Wrapf(err, "expected different type for opts field: `%s`", name)
		}
	}

	obj.Type = tOpts // type of opts struct, even an empty: `struct{}`
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *SchedulePolyFunc) Validate() error {
	// obj.Type can be nil if no 2nd arg is given, or a struct (even empty!)
	if obj.Type != nil && obj.Type.Kind != types.KindStruct { // build must be run first
		return fmt.Errorf("type must be nil or a struct")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *SchedulePolyFunc) Info() *interfaces.Info {
	typ := types.NewType("func(namespace str) []str") // simplest form
	if obj.Type != nil {
		typ = types.NewType(fmt.Sprintf("func(namespace str, opts %s) []str", obj.Type.String()))
	}
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		// output is list of hostnames chosen
		Sig: typ, // func kind
		Err: obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *SchedulePolyFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan *schedulerResult)
	obj.closeChan = make(chan struct{})
	//obj.init.Debug = true // use this for local debugging
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *SchedulePolyFunc) Stream() error {
	defer close(obj.init.Output) // the sender closes
	for {
		select {
		// TODO: should this first chan be run as a priority channel to
		// avoid some sort of glitch? is that even possible? can our
		// hostname check with reality (below) fix that?
		case input, ok := <-obj.init.Input:
			if !ok {
				obj.init.Input = nil // don't infinite loop back
				continue             // no more inputs, but don't return!
			}
			//if err := input.Type().Cmp(obj.Info().Sig.Input); err != nil {
			//	return errwrap.Wrapf(err, "wrong function input")
			//}

			if obj.last != nil && input.Cmp(obj.last) == nil {
				continue // value didn't change, skip it
			}
			obj.last = input // store for next

			namespace := input.Struct()["namespace"].Str()
			if namespace == "" {
				return fmt.Errorf("can't use an empty namespace")
			}

			opts := make(map[string]types.Value) // empty "struct"
			if val, exists := input.Struct()["opts"]; exists {
				opts = val.Struct()
			}

			if obj.init.Debug {
				obj.init.Logf("namespace: %s", namespace)
			}

			schedulerOpts := []scheduler.Option{}
			// don't add bad or zero-value options

			defaultStrategy := true
			if val, exists := opts["strategy"]; exists {
				if strategy := val.Str(); strategy != "" {
					if obj.init.Debug {
						obj.init.Logf("opts: strategy: %s", strategy)
					}
					defaultStrategy = false
					schedulerOpts = append(schedulerOpts, scheduler.StrategyKind(strategy))
				}
			}
			if defaultStrategy { // we always need to add one!
				schedulerOpts = append(schedulerOpts, scheduler.StrategyKind(DefaultStrategy))
			}
			if val, exists := opts["max"]; exists {
				// TODO: check for overflow
				if max := int(val.Int()); max > 0 {
					if obj.init.Debug {
						obj.init.Logf("opts: max: %d", max)
					}
					schedulerOpts = append(schedulerOpts, scheduler.MaxCount(max))
				}
			}
			if val, exists := opts["reuse"]; exists {
				reuse := val.Bool()
				if obj.init.Debug {
					obj.init.Logf("opts: reuse: %t", reuse)
				}
				schedulerOpts = append(schedulerOpts, scheduler.ReuseLease(reuse))
			}
			if val, exists := opts["ttl"]; exists {
				// TODO: check for overflow
				if ttl := int(val.Int()); ttl > 0 {
					if obj.init.Debug {
						obj.init.Logf("opts: ttl: %d", ttl)
					}
					schedulerOpts = append(schedulerOpts, scheduler.SessionTTL(ttl))
				}
			}

			// TODO: support changing the namespace over time...
			// TODO: possibly removing our stored value there first!
			if obj.namespace == "" {
				obj.namespace = namespace // store it

				if obj.init.Debug {
					obj.init.Logf("starting scheduler...")
				}
				var err error
				obj.scheduler, err = obj.init.World.Scheduler(obj.namespace, schedulerOpts...)
				if err != nil {
					return errwrap.Wrapf(err, "can't create scheduler")
				}

				// process the stream of scheduling output...
				go func() {
					defer close(obj.watchChan)
					ctx, cancel := context.WithCancel(context.Background())
					go func() {
						defer cancel() // unblock Next()
						defer obj.scheduler.Shutdown()
						select {
						case <-obj.closeChan:
							return
						}
					}()
					for {
						hosts, err := obj.scheduler.Next(ctx)
						select {
						case obj.watchChan <- &schedulerResult{
							hosts: hosts,
							err:   err,
						}:

						case <-obj.closeChan:
							return
						}
					}
				}()

			} else if obj.namespace != namespace {
				return fmt.Errorf("can't change namespace, previously: `%s`", obj.namespace)
			}

			continue // we send values on the watch chan, not here!

		case schedulerResult, ok := <-obj.watchChan:
			if !ok { // closed
				// XXX: maybe etcd reconnected? (fix etcd implementation)

				// XXX: if we close, perhaps the engine is
				// switching etcd hosts and we should retry?
				// maybe instead we should get an "etcd
				// reconnect" signal, and the lang will restart?
				return nil
			}
			if err := schedulerResult.err; err != nil {
				if err == scheduler.ErrEndOfResults {
					//return nil // TODO: we should probably fix the reconnect issue and use this here
					return fmt.Errorf("scheduler shutdown, reconnect bug?") // XXX: fix etcd reconnects
				}
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.namespace)
			}

			if obj.init.Debug {
				obj.init.Logf("got hosts: %+v", schedulerResult.hosts)
			}

			var result types.Value
			l := types.NewList(obj.Info().Sig.Out)
			for _, val := range schedulerResult.hosts {
				if err := l.Add(&types.StrValue{V: val}); err != nil {
					return errwrap.Wrapf(err, "list could not add val: `%s`", val)
				}
			}
			result = l // set list as result

			if obj.init.Debug {
				obj.init.Logf("result: %+v", result)
			}

			// if the result is still the same, skip sending an update...
			if obj.result != nil && result.Cmp(obj.result) == nil {
				continue // result didn't change
			}
			obj.result = result // store new result

		case <-obj.closeChan:
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-obj.closeChan:
			return nil
		}
	}
}

// Close runs some shutdown code for this function and turns off the stream.
func (obj *SchedulePolyFunc) Close() error {
	close(obj.closeChan)
	return nil
}

// schedulerResult combines our internal events into a single message packet.
type schedulerResult struct {
	hosts []string
	err   error
}

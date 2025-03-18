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

// test with:
// time ./mgmt run --hostname h1 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
// time ./mgmt run --hostname h2 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2381 --server-urls=http://127.0.0.1:2382 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
// time ./mgmt run --hostname h3 --seeds=http://127.0.0.1:2379 --client-urls=http://127.0.0.1:2383 --server-urls=http://127.0.0.1:2384 --tmp-prefix --no-pgp lang examples/lang/schedule0.mcl
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
	"sort"

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

	// DefaultStrategy is the strategy to use if none has been specified.
	DefaultStrategy = "rr"

	// StrictScheduleOpts specifies whether the opts passed into the
	// scheduler must be strictly what we're expecting, and nothing more.
	// If this was false, then we'd allow an opts struct that had a field
	// that wasn't used by the scheduler. This could be useful if we need to
	// migrate to a newer version of the function. It's probably best to
	// keep this strict.
	StrictScheduleOpts = true

	// arg names...
	scheduleArgNameNamespace = "namespace"
	scheduleArgNameOpts      = "opts"
)

func init() {
	funcs.ModuleRegister(ModuleName, ScheduleFuncName, func() interfaces.Func { return &ScheduleFunc{} })
}

var _ interfaces.BuildableFunc = &ScheduleFunc{} // ensure it meets this expectation

// ScheduleFunc is special function which determines where code should run in
// the cluster.
type ScheduleFunc struct {
	Type *types.Type // this is the type of opts used if specified

	built bool // was this function built yet?

	init *interfaces.Init

	args []types.Value

	world engine.SchedulerWorld

	namespace string
	scheduler *scheduler.Result

	last   types.Value
	result types.Value // last calculated output

	watchChan chan *schedulerResult
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ScheduleFunc) String() string {
	return ScheduleFuncName
}

// validOpts returns the available mapping of valid opts fields to types.
func (obj *ScheduleFunc) validOpts() map[string]*types.Type {
	return map[string]*types.Type{
		"strategy": types.TypeStr,
		"max":      types.TypeInt,
		"reuse":    types.TypeBool,
		"ttl":      types.TypeInt,
	}
}

// ArgGen returns the Nth arg name for this function.
func (obj *ScheduleFunc) ArgGen(index int) (string, error) {
	seq := []string{scheduleArgNameNamespace, scheduleArgNameOpts} // 2nd arg is optional
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *ScheduleFunc) sig() *types.Type {
	sig := types.NewType(fmt.Sprintf("func(%s str) []str", scheduleArgNameNamespace)) // simplest form
	if obj.Type != nil {
		sig = types.NewType(fmt.Sprintf("func(%s str, %s %s) []str", scheduleArgNameNamespace, scheduleArgNameOpts, obj.Type.String()))
	}
	return sig
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *ScheduleFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// func(namespace str) []str
	// OR
	// func(namespace str, opts ?1) []str

	if l := len(partialValues); l < 1 || l > 2 {
		return nil, nil, fmt.Errorf("must have at either one or two args")
	}

	var typ *types.Type
	if len(partialValues) == 1 {
		typ = types.NewType(fmt.Sprintf("func(%s str) []str", scheduleArgNameNamespace))
	}

	if len(partialValues) == 2 {
		typ = types.NewType(fmt.Sprintf("func(%s str, %s ?1) []str", scheduleArgNameNamespace, scheduleArgNameOpts))
	}

	return typ, []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *ScheduleFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 1 && len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the schedule function needs either one or two args")
	}
	if typ.Out == nil {
		return nil, fmt.Errorf("return type of function must be specified")
	}
	if typ.Map == nil {
		return nil, fmt.Errorf("invalid input type")
	}

	if err := typ.Out.Cmp(types.TypeListStr); err != nil {
		return nil, errwrap.Wrapf(err, "return type must be a list of strings")
	}

	tNamespace, exists := typ.Map[typ.Ord[0]]
	if !exists || tNamespace == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}

	if len(typ.Ord) == 1 {
		obj.Type = nil
		obj.built = true
		return obj.sig(), nil // done early, 2nd arg is absent!
	}
	tOpts, exists := typ.Map[typ.Ord[1]]
	if !exists || tOpts == nil {
		return nil, fmt.Errorf("second argument was missing")
	}

	if tOpts.Kind != types.KindStruct {
		return nil, fmt.Errorf("second argument must be of kind struct")
	}

	validOpts := obj.validOpts()

	if StrictScheduleOpts {
		// strict opts field checking!
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

	} else {
		// permissive field checking...
		validOptsSorted := []string{}
		for name := range validOpts {
			validOptsSorted = append(validOptsSorted, name)
		}
		sort.Strings(validOptsSorted)
		for _, name := range validOptsSorted {
			value := validOpts[name] // type

			t, exists := tOpts.Map[name]
			if !exists {
				continue // ignore it
			}

			// if it exists, check the type
			if err := t.Cmp(value); err != nil {
				return nil, errwrap.Wrapf(err, "expected different type for opts field: `%s`", name)
			}
		}
	}

	obj.Type = tOpts // type of opts struct, even an empty: `struct{}`
	obj.built = true
	return obj.sig(), nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *ScheduleFunc) Validate() error {
	if !obj.built {
		return fmt.Errorf("function wasn't built yet")
	}
	// obj.Type can be nil if no 2nd arg is given, or a struct (even empty!)
	if obj.Type != nil && obj.Type.Kind != types.KindStruct { // build must be run first
		return fmt.Errorf("type must be nil or a struct")
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ScheduleFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	var sig *types.Type
	if obj.built {
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: false, // definitely false
		Memo: false,
		// output is list of hostnames chosen
		Sig: sig, // func kind
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

	obj.watchChan = make(chan *schedulerResult)
	//obj.init.Debug = true // use this for local debugging
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ScheduleFunc) Stream(ctx context.Context) error {
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

			args, err := interfaces.StructToCallableArgs(input) // []types.Value, error)
			if err != nil {
				return err
			}
			obj.args = args

			namespace := args[0].Str()

			//namespace := input.Struct()[scheduleArgNameNamespace].Str()
			if namespace == "" {
				return fmt.Errorf("can't use an empty namespace")
			}

			opts := make(map[string]types.Value) // empty "struct"
			if val, exists := input.Struct()[scheduleArgNameOpts]; exists {
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
				obj.scheduler, err = obj.world.Scheduler(obj.namespace, schedulerOpts...)
				if err != nil {
					return errwrap.Wrapf(err, "can't create scheduler")
				}

				// process the stream of scheduling output...
				go func() {
					defer close(obj.watchChan)
					// XXX: maybe we could share the parent
					// ctx, but I have to work out the
					// ordering logic first. For now this is
					// just a port of what it was before.
					newCtx, cancel := context.WithCancel(context.Background())
					go func() {
						defer cancel() // unblock Next()
						defer obj.scheduler.Shutdown()
						select {
						case <-ctx.Done():
							return
						}
					}()
					for {
						hosts, err := obj.scheduler.Next(newCtx)
						select {
						case obj.watchChan <- &schedulerResult{
							hosts: hosts,
							err:   err,
						}:

						case <-ctx.Done():
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

		case <-ctx.Done():
			return nil
		}

		select {
		case obj.init.Output <- obj.result: // send
			// pass
		case <-ctx.Done():
			return nil
		}
	}
}

// schedulerResult combines our internal events into a single message packet.
type schedulerResult struct {
	hosts []string
	err   error
}

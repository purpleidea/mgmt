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
	"sort"

	"github.com/purpleidea/mgmt/etcd/scheduler" // TODO: is it okay to import this without abstraction?
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// DefaultStrategy is the strategy to use if none has been specified.
	DefaultStrategy = "rr"

	// StrictScheduleOpts specifies whether the opts passed into the
	// scheduler must be strictly what we're expecting, and nothing more.
	// If this was false, then we'd allow an opts struct that had a field
	// that wasn't used by the scheduler. This could be useful if we need to
	// migrate to a newer version of the function. It's probably best to
	// keep this strict.
	StrictScheduleOpts = true

	argNameNamespace = "namespace"
	argNameOpts      = "opts"
)

func init() {
	funcs.ModuleRegister(ModuleName, "schedule", func() interfaces.Func { return &SchedulePolyFunc{} })
}

// SchedulePolyFunc is special function which determines where code should run
// in the cluster.
type SchedulePolyFunc struct {
	Type *types.Type // this is the type of opts used if specified

	built bool // was this function built yet?

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

// ArgGen returns the Nth arg name for this function.
func (obj *SchedulePolyFunc) ArgGen(index int) (string, error) {
	seq := []string{argNameNamespace, argNameOpts} // 2nd arg is optional
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Unify returns the list of invariants that this func produces.
func (obj *SchedulePolyFunc) Unify(expr interfaces.Expr) ([]interfaces.Invariant, error) {
	var invariants []interfaces.Invariant
	var invar interfaces.Invariant

	// func(namespace str) []str
	// OR
	// func(namespace str, opts T1) []str

	namespaceName, err := obj.ArgGen(0)
	if err != nil {
		return nil, err
	}

	dummyNamespace := &interfaces.ExprAny{} // corresponds to the namespace type
	dummyOut := &interfaces.ExprAny{}       // corresponds to the out string

	// namespace arg type of string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyNamespace,
		Type: types.TypeStr,
	}
	invariants = append(invariants, invar)

	// return type of []string
	invar = &interfaces.EqualsInvariant{
		Expr: dummyOut,
		Type: types.NewType("[]str"),
	}
	invariants = append(invariants, invar)

	// generator function
	fn := func(fnInvariants []interfaces.Invariant, solved map[interfaces.Expr]*types.Type) ([]interfaces.Invariant, error) {
		for _, invariant := range fnInvariants {
			// search for this special type of invariant
			cfavInvar, ok := invariant.(*interfaces.CallFuncArgsValueInvariant)
			if !ok {
				continue
			}
			// did we find the mapping from us to ExprCall ?
			if cfavInvar.Func != expr {
				continue
			}
			// cfavInvar.Expr is the ExprCall!
			// cfavInvar.Args are the args that ExprCall uses!
			if len(cfavInvar.Args) == 0 {
				return nil, fmt.Errorf("unable to build function with no args")
			}
			if l := len(cfavInvar.Args); l > 2 {
				return nil, fmt.Errorf("unable to build function with %d args", l)
			}
			// we can either have one arg or two

			var invariants []interfaces.Invariant
			var invar interfaces.Invariant

			// add the relationships to the called args
			invar = &interfaces.EqualityInvariant{
				Expr1: cfavInvar.Args[0],
				Expr2: dummyNamespace,
			}
			invariants = append(invariants, invar)

			// first arg must be a string
			invar = &interfaces.EqualsInvariant{
				Expr: cfavInvar.Args[0],
				Type: types.TypeStr,
			}
			invariants = append(invariants, invar)

			// full function
			mapped := make(map[string]interfaces.Expr)
			ordered := []string{namespaceName}
			mapped[namespaceName] = dummyNamespace

			if len(cfavInvar.Args) == 2 { // two args is more complex
				dummyOpts := &interfaces.ExprAny{}

				optsTypeKnown := false

				// speculate about the type?
				if typ, err := cfavInvar.Args[1].Type(); err == nil {
					optsTypeKnown = true
					if typ.Kind != types.KindStruct {
						return nil, fmt.Errorf("second arg must be of kind struct")
					}

					// XXX: the problem is that I can't
					// currently express the opts struct as
					// an invariant, without building a big
					// giant, unusable exclusive...
					validOpts := obj.validOpts()

					if StrictScheduleOpts {
						// strict opts field checking!
						for _, name := range typ.Ord {
							t := typ.Map[name]
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

							t, exists := typ.Map[name]
							if !exists {
								continue // ignore it
							}

							// if it exists, check the type
							if err := t.Cmp(value); err != nil {
								return nil, errwrap.Wrapf(err, "expected different type for opts field: `%s`", name)
							}
						}
					}

					invar := &interfaces.EqualsInvariant{
						Expr: dummyOpts,
						Type: typ,
					}
					invariants = append(invariants, invar)
				}

				// If we're strict, require it, otherwise let
				// in whatever, and let Build() deal with it.
				if StrictScheduleOpts && !optsTypeKnown {
					return nil, fmt.Errorf("the type of the opts struct is not known")
				}

				// expression must match type of the input arg
				invar := &interfaces.EqualityInvariant{
					Expr1: dummyOpts,
					Expr2: cfavInvar.Args[1],
				}
				invariants = append(invariants, invar)

				mapped[argNameOpts] = dummyOpts
				ordered = append(ordered, argNameOpts)
			}

			invar = &interfaces.EqualityWrapFuncInvariant{
				Expr1:    expr, // maps directly to us!
				Expr2Map: mapped,
				Expr2Ord: ordered,
				Expr2Out: dummyOut,
			}
			invariants = append(invariants, invar)

			// TODO: do we return this relationship with ExprCall?
			invar = &interfaces.EqualityWrapCallInvariant{
				// TODO: should Expr1 and Expr2 be reversed???
				Expr1: cfavInvar.Expr,
				//Expr2Func: cfavInvar.Func, // same as below
				Expr2Func: expr,
			}
			invariants = append(invariants, invar)

			// TODO: are there any other invariants we should build?
			return invariants, nil // generator return
		}
		// We couldn't tell the solver anything it didn't already know!
		return nil, fmt.Errorf("couldn't generate new invariants")
	}
	invar = &interfaces.GeneratorInvariant{
		Func: fn,
	}
	invariants = append(invariants, invar)

	return invariants, nil
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
		obj.built = true
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

	if StrictScheduleOpts {
		// strict opts field checking!
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
				return errwrap.Wrapf(err, "expected different type for opts field: `%s`", name)
			}
		}
	}

	obj.Type = tOpts // type of opts struct, even an empty: `struct{}`
	obj.built = true
	return nil
}

// Validate tells us if the input struct takes a valid form.
func (obj *SchedulePolyFunc) Validate() error {
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
func (obj *SchedulePolyFunc) Info() *interfaces.Info {
	// It's important that you don't return a non-nil sig if this is called
	// before you're built. Type unification may call it opportunistically.
	var typ *types.Type
	if obj.built {
		typ = types.NewType("func(namespace str) []str") // simplest form
		if obj.Type != nil {
			typ = types.NewType(fmt.Sprintf("func(namespace str, opts %s) []str", obj.Type.String()))
		}
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

			namespace := input.Struct()[argNameNamespace].Str()
			if namespace == "" {
				return fmt.Errorf("can't use an empty namespace")
			}

			opts := make(map[string]types.Value) // empty "struct"
			if val, exists := input.Struct()[argNameOpts]; exists {
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

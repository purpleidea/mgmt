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

package core

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// CollectFuncName is the name this function is registered as. This
	// starts with an underscore so that it cannot be used from the lexer.
	CollectFuncName = funcs.CollectFuncName

	// arg names...
	collectArgNameKind  = "kind"
	collectArgNameNames = "names"

	//collectFuncInType = "[]struct{kind str; name str; host str}"
	//collectFuncInFieldKind = "kind" // must match above struct field
	collectFuncInFieldName = funcs.CollectFuncInFieldName
	collectFuncInFieldHost = funcs.CollectFuncInFieldHost

	// collectFuncInType is the most complex of the three possible input
	// types. The other two possible ones are str or []str.
	collectFuncInType = funcs.CollectFuncInType // "[]struct{name str; host str}"

	collectFuncOutFieldName = funcs.CollectFuncOutFieldName
	collectFuncOutFieldHost = funcs.CollectFuncOutFieldHost
	collectFuncOutFieldData = funcs.CollectFuncOutFieldData

	// collectFuncOutStruct is the struct type that we return a list of.
	collectFuncOutStruct = funcs.CollectFuncOutStruct

	// collectFuncOutType is the expected return type, the data field is an
	// encoded resource blob.
	// XXX: Once structs can be real map keys in mcl, could this instead be:
	// map{struct{name str; host str}: str} // key => $data (efficiency!)
	collectFuncOutType = funcs.CollectFuncOutType // "[]struct{name str; host str; data str}"
)

func init() {
	funcs.Register(CollectFuncName, func() interfaces.Func { return &CollectFunc{} }) // must register the func and name
}

var _ interfaces.InferableFunc = &CollectFunc{} // ensure it meets this expectation

// CollectFunc is a special internal function which gets given information about
// incoming resource collection data. For example, to collect, that "pseudo
// resource" will need to know what resource "kind" it's collecting, the names
// of those resources, and the corresponding hostnames that they are getting the
// data from. With that three-tuple of data, it can pull all of that from etcd
// and pass it into a hidden resource body field so that the collect "pseudo
// resource" can use it to build the exported resource!
//
// The "kind" comes in as the first arg. The second arg (in its complex form) is
// []struct{name str; host str} is what the end user is _asking_ this function
// for.
// TODO: We could have a second version of this collect function which takes a
// single arg which receives []struct{kind str; name str; host str} which would
// let us write a truly dynamic collector. It's unlikely we want to allow this
// in most cases because it lets you play type games since the field name in one
// resource kind might be a different type in another.
type CollectFunc struct {
	// Type is the type of the second arg that we receive. (When known.)
	Type *types.Type

	init *interfaces.Init

	input chan string // stream of inputs
	kind  *string     // the active kind

	watchChan chan error
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *CollectFunc) String() string {
	return CollectFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *CollectFunc) ArgGen(index int) (string, error) {
	seq := []string{collectArgNameKind, collectArgNameNames}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *CollectFunc) sig() *types.Type {
	arg := "?1"
	if obj.Type != nil {
		arg = obj.Type.String()
	}

	return types.NewType(fmt.Sprintf(
		"func(%s str, %s %s) %s",
		collectArgNameKind,
		collectArgNameNames,
		arg,
		collectFuncOutType,
	))
}

// check determines if our arg type is valid.
func (obj *CollectFunc) check(typ *types.Type) error {
	if typ.Cmp(types.TypeStr) == nil {
		return nil
	}
	if typ.Cmp(types.TypeListStr) == nil {
		return nil
	}
	if typ.Cmp(types.NewType(collectFuncInType)) == nil {
		return nil
	}

	return fmt.Errorf("unexpected type: %s", typ.String())
}

// FuncInfer takes partial type and value information from the call site of this
// function so that it can build an appropriate type signature for it. The type
// signature may include unification variables.
func (obj *CollectFunc) FuncInfer(partialType *types.Type, partialValues []types.Value) (*types.Type, []*interfaces.UnificationInvariant, error) {
	// There are many variants which we could allow... These variants are
	// what the user specifies in the $name field when they collect. They
	// will often get the third form from helper functions that filter the
	// data from the world graph, so that they can programmatically match
	// using our mcl language rather than hard-coding a mini matcher lang.
	//
	// XXX: Do we want to allow all these variants?
	//
	// func(str, str) out # matches all hostnames
	// OR
	// func(str, []str) out # matches all hostnames
	// OR
	// func(str, []struct{name str; host str} ) out # matches exact tuples or all hostnames if host is ""
	// SO
	// func(str, ?1) out
	// AND
	// out = []struct{name str; host str; data str} # it could have kind too, but not needed right now
	//
	// NOTE: map[str]str (name => host) is NOT a good choice because even
	// though we nominally have one host exporting a given name, it's valid
	// to have that same name come from more than one host and for them to
	// be compatible, almost like an "exported resources redundancy".
	//
	// NOTE map[str][]str (name => []host) is sensible, BUT it makes it
	// harder to express that we want "every host", which we can do with the
	// struct variant above by having host be the empty string. It's also
	// easier for the mcl programmer to understand that variant.

	if l := 2; len(partialValues) != l {
		return nil, nil, fmt.Errorf("function must have %d args", l)
	}
	if err := partialValues[0].Type().Cmp(types.TypeStr); err != nil {
		return nil, nil, errwrap.Wrapf(err, "function arg kind must be a str")
	}
	kind := partialValues[0].Str() // must not panic
	if kind == "" {
		return nil, nil, fmt.Errorf("function must not have an empty kind arg")
	}
	if !engine.IsKind(kind) {
		return nil, nil, fmt.Errorf("invalid resource kind: %s", kind)
	}

	// If second arg is one of what we're expecting, then we are solved!
	if len(partialType.Ord) == 2 && partialType.Map[partialType.Ord[1]] != nil {
		typ := partialType.Map[partialType.Ord[1]]
		if err := obj.check(typ); err == nil {
			obj.Type = typ // success!
		}
	}

	return obj.sig(), []*interfaces.UnificationInvariant{}, nil
}

// Build is run to turn the polymorphic, undetermined function, into the
// specific statically typed version. It is usually run after Unify completes,
// and must be run before Info() and any of the other Func interface methods are
// used. This function is idempotent, as long as the arg isn't changed between
// runs.
func (obj *CollectFunc) Build(typ *types.Type) (*types.Type, error) {
	// typ is the KindFunc signature we're trying to build...
	if typ.Kind != types.KindFunc {
		return nil, fmt.Errorf("input type must be of kind func")
	}

	if len(typ.Ord) != 2 {
		return nil, fmt.Errorf("the collect function needs two args")
	}
	tStr, exists := typ.Map[typ.Ord[0]]
	if !exists || tStr == nil {
		return nil, fmt.Errorf("first arg must be specified")
	}
	if tStr.Cmp(types.TypeStr) != nil {
		return nil, fmt.Errorf("first arg must be a str")
	}

	tArg, exists := typ.Map[typ.Ord[1]]
	if !exists || tArg == nil {
		return nil, fmt.Errorf("second arg must be specified")
	}

	if err := obj.check(tArg); err != nil {
		return nil, err
	}

	obj.Type = tArg // store it!

	return obj.sig(), nil
}

// Copy is implemented so that the obj.Type value is not lost if we copy this
// function. That value is learned during FuncInfer, and previously would have
// been lost by the time we used it in Build.
func (obj *CollectFunc) Copy() interfaces.Func {
	return &CollectFunc{
		Type: obj.Type, // don't copy because we use this after unification

		init: obj.init, // likely gets overwritten anyways
	}
}

// Validate tells us if the input struct takes a valid form.
func (obj *CollectFunc) Validate() error {
	if obj.Type == nil {
		return fmt.Errorf("the Type is unknown")
	}
	if err := obj.check(obj.Type); err != nil {
		return err
	}
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *CollectFunc) Info() *interfaces.Info {
	// Since this function implements FuncInfer we want sig to return nil to
	// avoid an accidental return of unification variables when we should be
	// getting them from FuncInfer, and not from here. (During unification!)
	var sig *types.Type
	if obj.Type != nil && obj.check(obj.Type) == nil {
		sig = obj.sig() // helper
	}
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  sig,
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *CollectFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan string)
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *CollectFunc) Stream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watch when exiting
	for {
		select {
		// TODO: should this first chan be run as a priority channel to
		// avoid some sort of glitch? is that even possible? can our
		// hostname check with reality (below) fix that?
		case kind, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			if obj.kind != nil && *obj.kind == kind {
				continue // nothing changed
			}

			// TODO: support changing the key over time?
			if obj.kind == nil {
				obj.kind = &kind // store
				var err error
				//  Don't send a value right away, wait for the
				// first Watch startup event to get one!
				obj.watchChan, err = obj.init.World.ResWatch(ctx, kind) // watch for var changes
				if err != nil {
					return err
				}
				continue // we get values on the watch chan, not here!
			}

			if *obj.kind == kind {
				continue // skip duplicates
			}

			// *obj.kind != kind
			return fmt.Errorf("can't change kind, previously: `%s`", *obj.kind)

		case err, ok := <-obj.watchChan:
			if !ok { // closed
				// XXX: if we close, perhaps the engine is
				// switching etcd hosts and we should retry?
				// maybe instead we should get an "etcd
				// reconnect" signal, and the lang will restart?
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "channel watch failed on `%s`", *obj.kind)
			}

			if err := obj.init.Event(ctx); err != nil { // send event
				return err
			}

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time. This was previously getValue which gets the value
// we're looking for.
func (obj *CollectFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("not enough args")
	}
	kind := args[0].Str()
	if kind == "" {
		return nil, fmt.Errorf("resource kind is empty")
	}
	if !engine.IsKind(kind) {
		return nil, fmt.Errorf("invalid resource kind: %s", kind)
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	if obj.init.Debug {
		obj.init.Logf("kind: %s", kind)
	}

	select {
	case obj.input <- kind:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	filters := []*engine.ResFilter{}

	arg := args[1]
	typ := arg.Type()
	// Can be one of: str, []str, []struct{name str; host str} for matching.

	if typ.Cmp(types.TypeStr) == nil { // it must be a name only
		filter := &engine.ResFilter{
			Kind: kind,
			Name: arg.Str(),
			Host: "", // any
		}
		filters = append(filters, filter)
	}
	if typ.Cmp(types.TypeListStr) == nil {
		for _, x := range arg.List() {
			filter := &engine.ResFilter{
				Kind: kind,
				Name: x.Str(),
				Host: "", // any
			}
			filters = append(filters, filter)
		}
	}
	if typ.Cmp(types.NewType(collectFuncInType)) == nil {
		for _, x := range arg.List() {
			st, ok := x.(*types.StructValue)
			if !ok {
				// programming error
				return nil, fmt.Errorf("value is not a struct")
			}
			name, exists := st.Lookup(collectFuncInFieldName)
			if !exists {
				// programming error?
				return nil, fmt.Errorf("name field is missing")
			}
			host, exists := st.Lookup(collectFuncInFieldHost)
			if !exists {
				// programming error?
				return nil, fmt.Errorf("host field is missing")
			}

			filter := &engine.ResFilter{
				Kind: kind,
				Name: name.Str(),
				Host: host.Str(),
			}
			filters = append(filters, filter)
		}
	}

	list := types.NewList(obj.Info().Sig.Out) // collectFuncOutType

	if len(filters) == 0 {
		// If we have no filters, it means we're matching on nothing,
		// which happens if we've pre-filtered away all the resources
		// that we'd want to collect, so here we return absolutely zero!
		return list, nil
	}

	resOutput, err := obj.init.World.ResCollect(ctx, filters)
	if err != nil {
		return nil, err
	}

	for _, x := range resOutput {
		// programming error if any of these error...
		if x.Kind != kind {
			return nil, fmt.Errorf("unexpected kind: %s", x.Kind)
		}
		if x.Name == "" {
			return nil, fmt.Errorf("unexpected empty name")
		}
		if x.Host == "" {
			return nil, fmt.Errorf("unexpected empty host")
		}
		if x.Host == "*" { // safety check
			return nil, fmt.Errorf("unexpected star host")
		}
		if x.Data == "" {
			return nil, fmt.Errorf("unexpected empty data")
		}

		name := &types.StrValue{V: x.Name}
		host := &types.StrValue{V: x.Host} // from
		data := &types.StrValue{V: x.Data}

		st := types.NewStruct(types.NewType(collectFuncOutStruct))
		if err := st.Set(collectFuncOutFieldName, name); err != nil {
			return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", collectFuncOutFieldName, name)
		}
		if err := st.Set(collectFuncOutFieldHost, host); err != nil {
			return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", collectFuncOutFieldHost, host)
		}
		if err := st.Set(collectFuncOutFieldData, data); err != nil {
			return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", collectFuncOutFieldData, data)
		}

		if err := list.Add(st); err != nil { // XXX: improve perf of Add
			return nil, err
		}
	}

	return list, nil // put struct into interface type
}

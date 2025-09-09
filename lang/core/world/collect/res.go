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

package corecollect

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/engine"
	coreworld "github.com/purpleidea/mgmt/lang/core/world"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ResFuncName is the name this function is registered as.
	ResFuncName = "res"

	// arg names...
	resArgNameKind = "kind"

	// Might as well pull these field names for consistency between the two.
	resFuncOutFieldName = funcs.CollectFuncOutFieldName
	resFuncOutFieldHost = funcs.CollectFuncOutFieldHost

	// resFuncOutStruct is the struct type that we return a list of.
	resFuncOutStruct = "struct{" + resFuncOutFieldName + " str; " + resFuncOutFieldHost + " str}"

	// resFuncOutType is the expected return type.
	resFuncOutType = "[]" + resFuncOutStruct // "[]struct{name str; host str}"
)

func init() {
	funcs.ModuleRegister(coreworld.ModuleName+"/"+ModuleName, ResFuncName, func() interfaces.Func { return &ResFunc{} }) // must register the func and name
}

// ResFunc is a special function which returns information about available
// resource collection data. You specify the kind, and it tells you which of
// those are available and from which hosts.
//
// This function is a simplified version of the internal _collect function.
//
// TODO: We could have a second version of this res function which can take a
// filter as a second or third arg to attempt to reduce the amount of raw data
// that we have to filter out in mcl.
type ResFunc struct {
	init *interfaces.Init

	input chan string // stream of inputs
	kind  *string     // the active kind

	watchChan chan error
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ResFunc) String() string {
	return ResFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ResFunc) ArgGen(index int) (string, error) {
	seq := []string{resArgNameKind}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// helper
func (obj *ResFunc) sig() *types.Type {
	return types.NewType(fmt.Sprintf(
		"func(%s str) %s",
		resArgNameKind,
		resFuncOutType,
	))
}

// Validate tells us if the input struct takes a valid form.
func (obj *ResFunc) Validate() error {
	return nil
}

// Info returns some static info about itself. Build must be called before this
// will return correct data.
func (obj *ResFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false,
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ResFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan string)
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ResFunc) Stream(ctx context.Context) error {
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
func (obj *ResFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
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
	filter := &engine.ResFilter{
		Kind: kind,
		Name: "", // any
		Host: "", // any
	}
	filters = append(filters, filter)

	resOutput, err := obj.init.World.ResCollect(ctx, filters)
	if err != nil {
		return nil, err
	}

	list := types.NewList(obj.Info().Sig.Out) // resFuncOutType
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

		name := &types.StrValue{V: x.Name}
		host := &types.StrValue{V: x.Host} // from

		st := types.NewStruct(types.NewType(resFuncOutStruct))
		if err := st.Set(resFuncOutFieldName, name); err != nil {
			return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", resFuncOutFieldName, name)
		}
		if err := st.Set(resFuncOutFieldHost, host); err != nil {
			return nil, errwrap.Wrapf(err, "struct could not add field `%s`, val: `%s`", resFuncOutFieldHost, host)
		}

		if err := list.Add(st); err != nil { // XXX: improve perf of Add
			return nil, err
		}
	}

	return list, nil // put struct into interface type
}

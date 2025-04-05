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

	last   types.Value // last value received to use for diff
	args   []types.Value
	kind   string
	result types.Value // last calculated output

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
		Sig:  obj.sig(),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ResFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.watchChan = make(chan error) // XXX: sender should close this, but did I implement that part yet???
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ResFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watch when exiting
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

			kind := args[0].Str()
			if kind == "" {
				return fmt.Errorf("can't use an empty kind")
			}
			if obj.init.Debug {
				obj.init.Logf("kind: %s", kind)
			}

			// TODO: support changing the key over time?
			if obj.kind == "" {
				obj.kind = kind // store it
				var err error
				//  Don't send a value right away, wait for the
				// first Watch startup event to get one!
				obj.watchChan, err = obj.init.World.ResWatch(ctx, obj.kind) // watch for var changes
				if err != nil {
					return err
				}

			} else if obj.kind != kind {
				return fmt.Errorf("can't change kind, previously: `%s`", obj.kind)
			}

			continue // we get values on the watch chan, not here!

		case err, ok := <-obj.watchChan:
			if !ok { // closed
				// XXX: if we close, perhaps the engine is
				// switching etcd hosts and we should retry?
				// maybe instead we should get an "etcd
				// reconnect" signal, and the lang will restart?
				return nil
			}
			if err != nil {
				return errwrap.Wrapf(err, "channel watch failed on `%s`", obj.kind)
			}

			result, err := obj.Call(ctx, obj.args) // get the value...
			if err != nil {
				return err
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

// Call this function with the input args and return the value if it is possible
// to do so at this time. This was previously getValue which gets the value
// we're looking for.
func (obj *ResFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	kind := args[0].Str()
	if kind == "" {
		return nil, fmt.Errorf("resource kind is empty")
	}
	if !engine.IsKind(kind) {
		return nil, fmt.Errorf("invalid resource kind: %s", kind)
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

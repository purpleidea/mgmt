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

package coreos

import (
	"context"
	"fmt"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
	"github.com/purpleidea/mgmt/util/recwatch"

	"github.com/purpleidea/lsmod"
)

const (
	// ModinfoLoadedFuncName is the name this function is registered as.
	ModinfoLoadedFuncName = "modinfo_loaded"

	// arg names...
	modinfoLoadedArgNameModule = "module"

	// procModules is where the modules data comes from.
	procModules = "/proc/modules"
)

func init() {
	funcs.ModuleRegister(ModuleName, ModinfoLoadedFuncName, func() interfaces.Func { return &ModinfoLoadedFunc{} }) // must register the func and name
}

// ModinfoLoadedFunc is a function that determines if a linux module exists and
// is loaded. This is similar to what you can determine from the `lsmod`
// command. If the module does not even exist, this also returns false.
type ModinfoLoadedFunc struct {
	init *interfaces.Init
	last types.Value // last value received to use for diff

	modulename *string     // the active module name
	result     types.Value // last calculated output
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ModinfoLoadedFunc) String() string {
	return ModinfoLoadedFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ModinfoLoadedFunc) ArgGen(index int) (string, error) {
	seq := []string{modinfoLoadedArgNameModule}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ModinfoLoadedFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ModinfoLoadedFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // maybe false because the bool can change
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  types.NewType(fmt.Sprintf("func(%s str) bool", modinfoLoadedArgNameModule)),
	}
}

// Init runs some startup code for this function.
func (obj *ModinfoLoadedFunc) Init(init *interfaces.Init) error {
	obj.init = init
	return nil
}

// Stream returns the changing values that this func has over time.
func (obj *ModinfoLoadedFunc) Stream(ctx context.Context) error {
	defer close(obj.init.Output) // the sender closes

	// create new watcher
	// XXX: does this file produce inotify events?
	recWatcher := &recwatch.RecWatcher{
		Path:    procModules,
		Recurse: false,
		Opts: []recwatch.Option{
			recwatch.Logf(obj.init.Logf),
			recwatch.Debug(obj.init.Debug),
		},
	}
	if err := recWatcher.Init(); err != nil {
		return errwrap.Wrapf(err, "could not watch file")
	}
	defer recWatcher.Close()

	for {
		select {
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

			modulename := input.Struct()[modinfoLoadedArgNameModule].Str()
			// TODO: add check for empty string
			if obj.modulename != nil && *obj.modulename == modulename {
				continue // nothing changed
			}
			obj.modulename = &modulename

		case event, ok := <-recWatcher.Events():
			if !ok {
				return fmt.Errorf("no more events")
			}
			if err := event.Error; err != nil {
				return errwrap.Wrapf(err, "error event received")
			}

			if obj.last == nil {
				continue // still waiting for input values
			}

		case <-ctx.Done():
			return nil
		}

		args, err := interfaces.StructToCallableArgs(obj.last) // []types.Value, error)
		if err != nil {
			return err
		}

		result, err := obj.Call(ctx, args)
		if err != nil {
			return err
		}

		// if the result is still the same, skip sending an update...
		if obj.result != nil && result.Cmp(obj.result) == nil {
			continue // result didn't change
		}
		obj.result = result // store new result

		select {
		case obj.init.Output <- obj.result: // send
			// pass

		case <-ctx.Done():
			return nil
		}
	}
}

// Call this function with the input args and return the value if it is possible
// to do so at this time.
func (obj *ModinfoLoadedFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	modulename := args[0].Str()

	m, err := lsmod.LsMod()
	if err != nil {
		return nil, errwrap.Wrapf(err, "error reading modules")
	}

	// XXX: is there a difference between exists and loaded?
	_, exists := m[modulename]

	return &types.BoolValue{
		V: exists,
	}, nil
}

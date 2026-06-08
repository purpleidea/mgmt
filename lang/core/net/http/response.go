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

package corenethttp

import (
	"context"
	"fmt"
	"os"

	corenet "github.com/purpleidea/mgmt/lang/core/net"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// ResponseFuncName is the name this function is registered as.
	ResponseFuncName = "response"

	// arg names...
	responseArgNameUID = "uid"

	// struct field names...
	responseFieldNameStatus = "status"
	responseFieldNameOutput = "output"
)

func init() {
	funcs.ModuleRegister(corenet.ModuleName+"/"+ModuleName, ResponseFuncName, func() interfaces.Func { return &ResponseFunc{} })
}

var _ interfaces.StreamableFunc = &ResponseFunc{}

// ResponseFunc is a function that returns the live response data of an http
// client resource. It takes the uid (the name of the http:client resource) and
// returns a struct of {status int; output str} which changes over time as that
// resource downloads a file. The status is zero until something happens, -1 if
// the resource hit an engine-level error, or otherwise the HTTP status code
// (eg: 200). The output is the downloaded body, and it is read lazily from disk
// each time so that we don't keep a second copy of the data in memory. Use this
// function instead of os.readfile because: while you could just write the file
// to a known location and read it via os.readfile, that would be usually worse
// since that would see filesystem (inotify) style events when it's written,
// rather than the safer, internal event system which notifies only once, and
// when the actual (precise) change occurs.
type ResponseFunc struct {
	interfaces.Textarea

	init *interfaces.Init

	input chan string // stream of inputs
	uid   *string     // the active uid

	watchChan chan struct{}
}

// String returns a simple name for this function. This is needed so this struct
// can satisfy the pgraph.Vertex interface.
func (obj *ResponseFunc) String() string {
	return ResponseFuncName
}

// ArgGen returns the Nth arg name for this function.
func (obj *ResponseFunc) ArgGen(index int) (string, error) {
	seq := []string{responseArgNameUID}
	if l := len(seq); index >= l {
		return "", fmt.Errorf("index %d exceeds arg length of %d", index, l)
	}
	return seq[index], nil
}

// sig is a helper to return the static type signature of this function.
func (obj *ResponseFunc) sig() *types.Type {
	// func(uid str) struct{status int; output str}
	return types.NewType(fmt.Sprintf(
		"func(%s str) struct{%s int; %s str}",
		responseArgNameUID,
		responseFieldNameStatus,
		responseFieldNameOutput,
	))
}

// Validate makes sure we've built our struct properly. It is usually unused for
// normal functions that users can use directly.
func (obj *ResponseFunc) Validate() error {
	return nil
}

// Info returns some static info about itself.
func (obj *ResponseFunc) Info() *interfaces.Info {
	return &interfaces.Info{
		Pure: false, // depends on the local API
		Memo: false,
		Fast: false,
		Spec: false,
		Sig:  obj.sig(),
		Err:  obj.Validate(),
	}
}

// Init runs some startup code for this function.
func (obj *ResponseFunc) Init(init *interfaces.Init) error {
	obj.init = init
	obj.input = make(chan string)
	obj.watchChan = make(chan struct{}) // sender closes this when Stream ends
	return nil
}

// Copy is implemented so that the type value is not lost if we copy this
// function.
func (obj *ResponseFunc) Copy() interfaces.Func {
	return &ResponseFunc{
		Textarea: obj.Textarea,

		init: obj.init, // likely gets overwritten anyways
	}
}

// Stream returns the changing values that this func has over time.
func (obj *ResponseFunc) Stream(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // important so that we cleanup the watch when exiting
	for {
		select {
		case uid, ok := <-obj.input:
			if !ok {
				obj.input = nil // don't infinite loop back
				return fmt.Errorf("unexpected close")
			}

			if obj.uid != nil && *obj.uid == uid {
				continue // nothing changed
			}

			// We don't support changing the uid over time, since
			// the watch is set up against a single resource name.
			if obj.uid == nil {
				obj.uid = &uid // store it
				var err error
				// Don't send a value right away, wait for the
				// first HTTPWatch startup event to get one!
				obj.watchChan, err = obj.init.Local.HTTPWatch(ctx, uid)
				if err != nil {
					// rare, and not even possible today
					return err
				}
				continue // we get values on the watch chan, not here!
			}

			// *obj.uid != uid
			return fmt.Errorf("can't change uid, previously: `%s`", *obj.uid)

		case _, ok := <-obj.watchChan:
			if !ok { // closed
				return nil
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
// to do so at this time.
func (obj *ResponseFunc) Call(ctx context.Context, args []types.Value) (types.Value, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("not enough args")
	}
	uid := args[0].Str()
	if uid == "" {
		return nil, fmt.Errorf("can't use an empty uid")
	}

	// Check before we send to a chan where we'd need Stream to be running.
	if obj.init == nil {
		return nil, funcs.ErrCantSpeculate
	}

	if obj.init.Debug {
		obj.init.Logf("uid: %s", uid)
	}

	select {
	case obj.input <- uid:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	resp, err := obj.init.Local.HTTPGet(ctx, uid)
	if err != nil {
		// rare, and not even possible today
		return nil, err
	}
	if resp == nil {
		// programming error
		return nil, fmt.Errorf("received empty response")
	}

	// Read the downloaded body lazily from disk, but only when the resource
	// told us where to find a valid one.
	output := ""
	if resp.Path != "" {
		b, err := os.ReadFile(resp.Path)
		if err != nil {
			return nil, errwrap.Wrapf(err, "could not read http output for `%s`", uid)
		}
		output = string(b)
	}

	st := types.NewStruct(obj.sig().Out)
	if err := st.Set(responseFieldNameStatus, &types.IntValue{V: int64(resp.Status)}); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`", responseFieldNameStatus)
	}
	if err := st.Set(responseFieldNameOutput, &types.StrValue{V: output}); err != nil {
		return nil, errwrap.Wrapf(err, "struct could not add field `%s`", responseFieldNameOutput)
	}

	return st, nil
}

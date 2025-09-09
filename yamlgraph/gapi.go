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

package yamlgraph

import (
	"context"
	"fmt"
	"sync"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// Name is the name of this frontend.
	Name = "yaml"

	// Start is the entry point filename that we use. It is arbitrary.
	Start = "/start.yaml"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main yamlgraph GAPI interface.
type GAPI struct {
	InputURI string // input URI of file system containing yaml graph to use

	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
	err         error
	errMutex    *sync.Mutex // guards err
}

// Cli takes an *Info struct, and returns our deploy if activated, and if there
// are any validation problems, you should return an error. If there is no
// deploy, then you should return a nil deploy and a nil error.
func (obj *GAPI) Cli(info *gapi.Info) (*gapi.Deploy, error) {
	args, ok := info.Args.(*cliUtil.YamlArgs)
	if !ok {
		// programming error
		return nil, fmt.Errorf("could not convert to our struct")
	}

	fs := info.Fs
	//debug := info.Debug
	//logf := func(format string, v ...interface{}) {
	//	 info.Logf(Name + ": "+format, v...)
	//}

	writeableFS, ok := fs.(engine.WriteableFS)
	if !ok {
		return nil, fmt.Errorf("the FS was not writeable")
	}

	// single file input only
	if err := gapi.CopyFileToFs(writeableFS, args.Input, Start); err != nil {
		return nil, errwrap.Wrapf(err, "can't copy yaml from `%s` to `%s`", args.Input, Start)
	}

	return &gapi.Deploy{
		Name: Name,
		Noop: info.Flags.Noop,
		Sema: info.Flags.Sema,
		GAPI: &GAPI{
			InputURI: fs.URI(),
			// TODO: add properties here...
		},
	}, nil
}

// Init initializes the yamlgraph GAPI struct.
func (obj *GAPI) Init(data *gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.InputURI == "" {
		return fmt.Errorf("the InputURI param must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.errMutex = &sync.Mutex{}
	obj.initialized = true
	return nil
}

// Info returns some data about the GAPI implementation.
func (obj *GAPI) Info() *gapi.InfoResult {
	return &gapi.InfoResult{
		URI: obj.InputURI,
	}
}

// graph returns a current Graph.
func (obj *GAPI) graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	fs, err := obj.data.World.Fs(obj.InputURI) // open the remote file system
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't load yaml from file system `%s`", obj.InputURI)
	}

	b, err := fs.ReadFile(Start) // read the single file out of it
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read yaml from file `%s`", Start)
	}

	debug := obj.data.Debug
	logf := func(format string, v ...interface{}) {
		// TODO: add the Name prefix in parent logger
		obj.data.Logf(Name+": "+format, v...)
	}
	config, err := NewGraphConfigFromFile(b, debug, logf)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("%s: NewGraphConfigFromFile returned nil", Name)
	}

	g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, err
}

// Next returns nil errors every time there could be a new graph.
func (obj *GAPI) Next(ctx context.Context) chan gapi.Next {
	ch := make(chan gapi.Next)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			err := fmt.Errorf("%s: GAPI is not initialized", Name)
			next := gapi.Next{
				Err:  err,
				Exit: true, // exit, b/c programming error?
			}
			select {
			case ch <- next:
			case <-ctx.Done():
				obj.errAppend(ctx.Err())
				return
			}
			obj.errAppend(err)
			return
		}

		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		var watchChan chan error
		if obj.data.NoStreamWatch {
			watchChan = nil
		} else {
			// XXX: disabled for now until someone ports to new API.
			//var err error
			//watchChan, err = obj.data.World.ResWatch(ctx)
			//if err != nil {
			//	next := gapi.Next{
			//		Err:  errwrap.Wrapf(err, "%s: could not start watch", Name),
			//		Exit: true, // exit, b/c programming error?
			//	}
			//	ch <- next
			//	return
			//}
		}

		for {
			var err error
			var ok bool
			select {
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				// pass
			case err, ok = <-watchChan:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}

			obj.data.Logf("generating new graph...")
			g, err := obj.graph()
			if err != nil {
				obj.errAppend(err)
				return
			}

			next := gapi.Next{
				Graph: g,
				//Exit: true, // TODO: for permanent shutdown!
				Err: err,
			}
			select {
			case ch <- next: // trigger a run (send a msg)
				// TODO: if the error is really bad, we could:
				//if err != nil {
				//	return
				//}
			// unblock if we exit while waiting to send!
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// Err will contain the last error when Next shuts down. It waits for all the
// running processes to exit before it returns.
func (obj *GAPI) Err() error {
	obj.wg.Wait()
	return obj.err
}

// errAppend is a simple helper function.
func (obj *GAPI) errAppend(err error) {
	obj.errMutex.Lock()
	obj.err = errwrap.Append(obj.err, err)
	obj.errMutex.Unlock()
}

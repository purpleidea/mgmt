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

package empty

import (
	"fmt"
	"sync"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
)

const (
	// Name is the name of this frontend.
	Name = "empty"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main lang GAPI interface.
type GAPI struct {
	// Wait should be true if we don't use any existing (stale) deploys.
	// This means that if you start an empty GAPI, then it will immediately
	// try to look for and run any existing deploys that have been stored in
	// the cluster that it has connected to. If this is true, then it will
	// only start on the next deploy. To be honest, we should probably never
	// wait, but this was accidentally how it was initially implemented, so
	// we'll change the default and add this in as a flag for now. We may
	// remove this in the future unless someone has a good reason for
	// needing it.
	Wait bool

	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          *sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes an *Info struct, and returns our deploy if activated, and if there
// are any validation problems, you should return an error. If there is no
// deploy, then you should return a nil deploy and a nil error.
func (obj *GAPI) Cli(info *gapi.Info) (*gapi.Deploy, error) {
	args, ok := info.Args.(*cliUtil.EmptyArgs)
	if !ok {
		// programming error
		return nil, fmt.Errorf("could not convert to our struct")
	}

	return &gapi.Deploy{
		Name: Name,
		//Noop: false,
		GAPI: &GAPI{
			Wait: args.Wait,
		},
	}, nil
}

// Init initializes the lang GAPI struct.
func (obj *GAPI) Init(data *gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}
	obj.initialized = true
	return nil
}

// Info returns some data about the GAPI implementation.
func (obj *GAPI) Info() *gapi.InfoResult {
	return &gapi.InfoResult{
		URI: "",
	}
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	obj.data.Logf("generating empty graph...")
	g, err := pgraph.NewGraph("empty")
	if err != nil {
		return nil, err
	}

	return g, nil
}

// Next returns nil errors every time there could be a new graph.
func (obj *GAPI) Next() chan gapi.Next {
	ch := make(chan gapi.Next)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			next := gapi.Next{
				Err:  fmt.Errorf("%s: GAPI is not initialized", Name),
				Exit: true, // exit, b/c programming error?
			}
			select {
			case ch <- next:
			case <-obj.closeChan:
			}
			return
		}

		// send only one event
		next := gapi.Next{
			Exit: false,
			Err:  nil,
		}
		select {
		case ch <- next: // trigger a run (send a msg)
			// pass

		// unblock if we exit while waiting to send!
		case <-obj.closeChan:
			return
		}
	}()
	return ch
}

// Close shuts down the lang GAPI.
func (obj *GAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("%s: GAPI is not initialized", Name)
	}
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

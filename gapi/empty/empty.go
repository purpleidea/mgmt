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

package empty

import (
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"

	"github.com/urfave/cli"
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
	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          *sync.WaitGroup // sync group for tunnel go routines
}

// CliFlags returns a list of flags used by the specified subcommand.
func (obj *GAPI) CliFlags(command string) []cli.Flag {
	return []cli.Flag{}
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(*gapi.CliInfo) (*gapi.Deploy, error) {
	return &gapi.Deploy{
		Name: Name,
		//Noop: false,
		GAPI: &GAPI{},
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

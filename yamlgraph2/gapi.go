// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package yamlgraph2

import (
	"fmt"
	"log"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/recwatch"
)

// GAPI implements the main yamlgraph GAPI interface.
type GAPI struct {
	File *string // yaml graph definition to use; nil if undefined

	data          gapi.Data
	initialized   bool
	closeChan     chan struct{}
	wg            sync.WaitGroup // sync group for tunnel go routines
	configWatcher *recwatch.ConfigWatcher
}

// NewGAPI creates a new yamlgraph GAPI struct and calls Init().
func NewGAPI(data gapi.Data, file *string) (*GAPI, error) {
	obj := &GAPI{
		File: file,
	}
	return obj, obj.Init(data)
}

// Init initializes the yamlgraph GAPI struct.
func (obj *GAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.File == nil {
		return fmt.Errorf("the File param must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	obj.configWatcher = recwatch.NewConfigWatcher()
	return nil
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("yamlgraph: GAPI is not initialized")
	}

	config := ParseConfigFromFile(*obj.File)
	if config == nil {
		return nil, fmt.Errorf("yamlgraph: ParseConfigFromFile returned nil")
	}

	g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, err
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
				Err:  fmt.Errorf("yamlgraph: GAPI is not initialized"),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		watchChan, configChan := make(chan error), make(chan error)
		if obj.data.NoConfigWatch {
			configChan = nil
		} else {
			configChan = obj.configWatcher.ConfigWatch(*obj.File) // simple
		}
		if obj.data.NoStreamWatch {
			watchChan = nil
		} else {
			watchChan = obj.data.World.ResWatch()
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
			case err, ok = <-configChan: // returns nil events on ok!
				if !ok { // the channel closed!
					return
				}
			case <-obj.closeChan:
				return
			}

			log.Printf("yamlgraph: Generating new graph...")
			next := gapi.Next{
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
			case <-obj.closeChan:
				return
			}
		}
	}()
	return ch
}

// Close shuts down the yamlgraph GAPI.
func (obj *GAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("yamlgraph: GAPI is not initialized")
	}
	obj.configWatcher.Close()
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

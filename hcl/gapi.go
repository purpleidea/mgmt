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

package hcl

import (
	"fmt"
	"log"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/recwatch"
)

// GAPI ...
type GAPI struct {
	File *string

	initialized   bool
	data          gapi.Data
	wg            sync.WaitGroup
	closeChan     chan struct{}
	configWatcher *recwatch.ConfigWatcher
}

// NewGAPI ...
func NewGAPI(data gapi.Data, file *string) (*GAPI, error) {
	if file == nil {
		return nil, fmt.Errorf("empty file given")
	}

	obj := &GAPI{
		File: file,
	}
	return obj, obj.Init(data)
}

// Init ...
func (obj *GAPI) Init(d gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}

	if obj.File == nil {
		return fmt.Errorf("file cannot be nil")
	}

	obj.data = d
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	obj.configWatcher = recwatch.NewConfigWatcher()

	return nil
}

// Graph ...
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	config, err := loadHcl(obj.File)
	if err != nil {
		return nil, fmt.Errorf("unable to parse graph: %s", err)
	}

	return graphFromConfig(config, obj.data)
}

// Next ...
func (obj *GAPI) Next() chan gapi.Next {
	ch := make(chan gapi.Next)
	obj.wg.Add(1)

	go func() {
		defer obj.wg.Done()
		defer close(ch)
		if !obj.initialized {
			next := gapi.Next{
				Err:  fmt.Errorf("hcl: GAPI is not initialized"),
				Exit: true,
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
			case <-startChan:
				startChan = nil
			case err, ok = <-watchChan:
			case err, ok = <-configChan:
				if !ok {
					return
				}
			case <-obj.closeChan:
				return
			}

			log.Printf("hcl: generating new graph")
			next := gapi.Next{
				Err: err,
			}

			select {
			case ch <- next:
			case <-obj.closeChan:
				return
			}
		}
	}()

	return ch
}

// Close ...
func (obj *GAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("hcl: GAPI is not initialized")
	}

	obj.configWatcher.Close()
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false
	return nil
}

// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
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

package puppet

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
)

// GAPI implements the main puppet GAPI interface.
type GAPI struct {
	PuppetParam *string // puppet mode to run; nil if undefined
	PuppetConf  string  // the path to an alternate puppet.conf file

	data        gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// NewGAPI creates a new puppet GAPI struct and calls Init().
func NewGAPI(data gapi.Data, puppetParam *string, puppetConf string) (*GAPI, error) {
	obj := &GAPI{
		PuppetParam: puppetParam,
		PuppetConf:  puppetConf,
	}
	return obj, obj.Init(data)
}

// Init initializes the puppet GAPI struct.
func (obj *GAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("Already initialized!")
	}
	if obj.PuppetParam == nil {
		return fmt.Errorf("The PuppetParam param must be specified!")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("Puppet: GAPI is not initialized!")
	}
	config := ParseConfigFromPuppet(*obj.PuppetParam, obj.PuppetConf)
	if config == nil {
		return nil, fmt.Errorf("Puppet: ParseConfigFromPuppet returned nil!")
	}
	g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.EmbdEtcd, obj.data.Noop)
	return g, err
}

// SwitchStream returns nil errors every time there could be a new graph.
func (obj *GAPI) SwitchStream() chan error {
	if obj.data.NoWatch {
		return nil
	}
	puppetChan := func() <-chan time.Time { // helper function
		return time.Tick(time.Duration(PuppetInterval(obj.PuppetConf)) * time.Second)
	}
	ch := make(chan error)
	obj.wg.Add(1)
	go func() {
		defer obj.wg.Done()
		defer close(ch) // this will run before the obj.wg.Done()
		if !obj.initialized {
			ch <- fmt.Errorf("Puppet: GAPI is not initialized!")
			return
		}
		pChan := puppetChan()
		for {
			select {
			case _, ok := <-pChan:
				if !ok { // the channel closed!
					return
				}
				log.Printf("Puppet: Generating new graph...")
				pChan = puppetChan() // TODO: okay to update interval in case it changed?
				ch <- nil            // trigger a run
			case <-obj.closeChan:
				return
			}
		}
	}()
	return ch
}

// Close shuts down the Puppet GAPI.
func (obj *GAPI) Close() error {
	if !obj.initialized {
		return fmt.Errorf("Puppet: GAPI is not initialized!")
	}
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

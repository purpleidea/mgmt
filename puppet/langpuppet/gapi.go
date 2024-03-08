// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package langpuppet

import (
	"fmt"
	"sync"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/gapi"
	lang "github.com/purpleidea/mgmt/lang/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/puppet"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// Name is the name of this frontend.
	Name = "langpuppet"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main langpuppet GAPI interface. It wraps the Puppet and
// Lang GAPIs and receives graphs from both. It then runs a merging algorithm
// that mainly just makes a union of both the sets of vertices and edges. Some
// vertices are merged using a naming convention. Details can be found in the
// langpuppet.mergeGraphs function.
type GAPI struct {
	// the wrapped lang entrypoint
	langGAPI gapi.GAPI
	// the wrapped puppet entrypoint
	puppetGAPI gapi.GAPI

	// the most recent graph received from lang
	currentLangGraph *pgraph.Graph
	// the most recent graph received from puppet
	currentPuppetGraph *pgraph.Graph

	// flag to indicate that a new graph from lang is ready
	langGraphReady bool
	// flag to indicate that a new graph from puppet is ready
	puppetGraphReady bool
	graphFlagMutex   *sync.Mutex

	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes an *Info struct, and returns our deploy if activated, and if there
// are any validation problems, you should return an error. If there is no
// deploy, then you should return a nil deploy and a nil error.
func (obj *GAPI) Cli(info *gapi.Info) (*gapi.Deploy, error) {
	args, ok := info.Args.(*cliUtil.LangPuppetArgs)
	if !ok {
		// programming error
		return nil, fmt.Errorf("could not convert to our struct")
	}
	flags := info.Flags
	fs := info.Fs
	debug := info.Debug
	logf := info.Logf

	langInfo := &gapi.Info{
		Args: &cliUtil.LangArgs{
			Input:        args.LangInput,
			Download:     args.Download,
			OnlyDownload: args.OnlyDownload,
			Update:       args.Update,
			OnlyUnify:    args.OnlyUnify,
			SkipUnify:    args.SkipUnify,
			Depth:        args.Depth,
			Retry:        args.Retry,
			ModulePath:   args.ModulePath,
		},
		Flags: flags,
		Fs:    fs,
		Debug: debug,
		Logf:  logf, // TODO: wrap logf?
	}
	puppetInfo := &gapi.Info{
		Args: &cliUtil.PuppetArgs{
			Input:      args.PuppetInput,
			PuppetConf: args.PuppetConf,
		},
		Flags: flags,
		Fs:    fs,
		Debug: debug,
		Logf:  logf, // TODO: wrap logf?
	}

	var langDeploy *gapi.Deploy
	var puppetDeploy *gapi.Deploy
	var err error

	if langDeploy, err = (&lang.GAPI{}).Cli(langInfo); err != nil {
		return nil, err
	}
	if puppetDeploy, err = (&puppet.GAPI{}).Cli(puppetInfo); err != nil {
		return nil, err
	}

	return &gapi.Deploy{
		Name: Name,
		Noop: info.Flags.Noop,
		Sema: info.Flags.Sema,
		GAPI: &GAPI{
			langGAPI:   langDeploy.GAPI,
			puppetGAPI: puppetDeploy.GAPI,
		},
	}, nil
}

// Init initializes the langpuppet GAPI struct.
func (obj *GAPI) Init(data *gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	obj.data = data // store for later
	obj.graphFlagMutex = &sync.Mutex{}

	dataLang := &gapi.Data{
		Program:       obj.data.Program,
		Version:       obj.data.Version,
		Hostname:      obj.data.Hostname,
		World:         obj.data.World,
		Noop:          obj.data.Noop,
		NoStreamWatch: obj.data.NoStreamWatch,
		Debug:         obj.data.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.data.Logf(lang.Name+": "+format, v...)
		},
	}
	dataPuppet := &gapi.Data{
		Program:       obj.data.Program,
		Version:       obj.data.Version,
		Hostname:      obj.data.Hostname,
		World:         obj.data.World,
		Noop:          obj.data.Noop,
		NoStreamWatch: obj.data.NoStreamWatch,
		Debug:         obj.data.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.data.Logf(puppet.Name+": "+format, v...)
		},
	}

	if err := obj.langGAPI.Init(dataLang); err != nil {
		return err
	}
	if err := obj.puppetGAPI.Init(dataPuppet); err != nil {
		return err
	}

	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	var err error
	obj.graphFlagMutex.Lock()
	if obj.langGraphReady {
		obj.langGraphReady = false
		obj.graphFlagMutex.Unlock()
		obj.currentLangGraph, err = obj.langGAPI.Graph()
		if err != nil {
			return nil, err
		}
	} else {
		obj.graphFlagMutex.Unlock()
	}

	obj.graphFlagMutex.Lock()
	if obj.puppetGraphReady {
		obj.puppetGraphReady = false
		obj.graphFlagMutex.Unlock()
		obj.currentPuppetGraph, err = obj.puppetGAPI.Graph()
		if err != nil {
			return nil, err
		}
	} else {
		obj.graphFlagMutex.Unlock()
	}

	g, err := mergeGraphs(obj.currentLangGraph, obj.currentPuppetGraph)

	if obj.data.Debug {
		obj.currentLangGraph.Logf(func(format string, v ...interface{}) {
			obj.data.Logf("graph: "+lang.Name+": "+format, v...)
		})
		obj.currentPuppetGraph.Logf(func(format string, v ...interface{}) {
			obj.data.Logf("graph: "+puppet.Name+": "+format, v...)
		})
		if err == nil {
			g.Logf(func(format string, v ...interface{}) {
				obj.data.Logf("graph: "+Name+": "+format, v...)
			})
		}
	}

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
				Err:  fmt.Errorf("%s: GAPI is not initialized", Name),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}
		nextLang := obj.langGAPI.Next()
		nextPuppet := obj.puppetGAPI.Next()

		firstLang := false
		firstPuppet := false

		for {
			var err error
			exit := false
			select {
			case nextChild := <-nextLang:
				if nextChild.Err != nil {
					err = nextChild.Err
					exit = nextChild.Exit
				} else {
					obj.graphFlagMutex.Lock()
					obj.langGraphReady = true
					obj.graphFlagMutex.Unlock()
					firstLang = true
				}
			case nextChild := <-nextPuppet:
				if nextChild.Err != nil {
					err = nextChild.Err
					exit = nextChild.Exit
				} else {
					obj.graphFlagMutex.Lock()
					obj.puppetGraphReady = true
					obj.graphFlagMutex.Unlock()
					firstPuppet = true
				}
			case <-obj.closeChan:
				return
			}

			if (!firstLang || !firstPuppet) && err == nil {
				continue
			}

			if err == nil {
				obj.data.Logf("generating new composite graph...")
			}
			next := gapi.Next{
				Exit: exit,
				Err:  err,
			}

			select {
			case ch <- next: // trigger a run (send a msg)
			// unblock if we exit while waiting to send!
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
		return fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	var err error
	e1 := obj.langGAPI.Close()
	err = errwrap.Append(err, errwrap.Wrapf(e1, "closing lang GAPI failed"))

	e2 := obj.puppetGAPI.Close()
	err = errwrap.Append(err, errwrap.Wrapf(e2, "closing Puppet GAPI failed"))

	close(obj.closeChan)
	obj.initialized = false // closed = true
	return err
}

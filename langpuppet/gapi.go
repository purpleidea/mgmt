// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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

package langpuppet

import (
	"flag"
	"fmt"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	lang "github.com/purpleidea/mgmt/lang/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/puppet"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/urfave/cli/v2"
)

const (
	// Name is the name of this frontend.
	Name = "langpuppet"
	// FlagPrefix gets prepended to each flag of both the puppet and lang GAPI.
	FlagPrefix = "lp-"
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
	langGAPI   gapi.GAPI // the wrapped lang entrypoint
	puppetGAPI gapi.GAPI // the wrapped puppet entrypoint

	currentLangGraph   *pgraph.Graph // the most recent graph received from lang
	currentPuppetGraph *pgraph.Graph // the most recent graph received from puppet

	langGraphReady   bool // flag to indicate that a new graph from lang is ready
	puppetGraphReady bool // flag to indicate that a new graph from puppet is ready
	graphFlagMutex   *sync.Mutex

	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// CliFlags returns a list of flags used by this deploy subcommand. It consists
// of all flags accepted by lang and puppet mode, with a respective "lp-"
// prefix.
func (obj *GAPI) CliFlags(command string) []cli.Flag {
	langFlags := (&lang.GAPI{}).CliFlags(command)
	puppetFlags := (&puppet.GAPI{}).CliFlags(command)

	l := &cli.StringFlag{
		Name:  fmt.Sprintf("%s, %s", lang.Name, lang.Name[0:1]),
		Value: "",
		Usage: "code to deploy",
	}
	langFlags = append(langFlags, l)
	p := &cli.StringFlag{
		Name:  fmt.Sprintf("%s, %s", puppet.Name, puppet.Name[0:1]),
		Value: "",
		Usage: "load graph from puppet, optionally takes a manifest or path to manifest file",
	}
	puppetFlags = append(puppetFlags, p)

	var childFlags []cli.Flag
	for _, flag := range append(langFlags, puppetFlags...) {
		childFlags = append(childFlags, &cli.StringFlag{
			Name:  FlagPrefix + flag.Names()[0],
			Value: "",
			Usage: fmt.Sprintf("equivalent for '%s' when using the lang/puppet entrypoint", flag.Names()[0]),
		})
	}

	return childFlags
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(cliInfo *gapi.CliInfo) (*gapi.Deploy, error) {
	c := cliInfo.CliContext
	fs := cliInfo.Fs // copy files from local filesystem *into* this fs...
	debug := cliInfo.Debug
	logf := func(format string, v ...interface{}) {
		cliInfo.Logf(Name+": "+format, v...)
	}

	if !c.IsSet(FlagPrefix+lang.Name) && !c.IsSet(FlagPrefix+puppet.Name) {
		return nil, nil
	}

	if !c.IsSet(FlagPrefix+lang.Name) || c.String(FlagPrefix+lang.Name) == "" {
		return nil, fmt.Errorf("%s input is empty", FlagPrefix+lang.Name)
	}
	if !c.IsSet(FlagPrefix+puppet.Name) || c.String(FlagPrefix+puppet.Name) == "" {
		return nil, fmt.Errorf("%s input is empty", FlagPrefix+puppet.Name)
	}

	flagSet := flag.NewFlagSet(Name, flag.ContinueOnError)

	for _, flag := range c.FlagNames() {
		if !c.IsSet(flag) {
			continue
		}
		childFlagName := strings.TrimPrefix(flag, FlagPrefix)
		flagSet.String(childFlagName, "", "no usage string needed here")
		flagSet.Set(childFlagName, c.String(flag))
	}

	var langDeploy *gapi.Deploy
	var puppetDeploy *gapi.Deploy
	// XXX: put the c.String(FlagPrefix+lang.Name) into the argv here!
	langCliInfo := &gapi.CliInfo{
		CliContext: cli.NewContext(c.App, flagSet, c.Lineage()[1]),
		Fs:         fs,
		Debug:      debug,
		Logf:       logf, // TODO: wrap logf?
	}
	// XXX: put the c.String(FlagPrefix+puppet.Name) into the argv here!
	puppetCliInfo := &gapi.CliInfo{
		CliContext: cli.NewContext(c.App, flagSet, c.Lineage()[1]),
		Fs:         fs,
		Debug:      debug,
		Logf:       logf, // TODO: wrap logf?
	}
	var err error

	// we don't really need the deploy object from the child GAPIs
	if langDeploy, err = (&lang.GAPI{}).Cli(langCliInfo); err != nil {
		return nil, err
	}
	if puppetDeploy, err = (&puppet.GAPI{}).Cli(puppetCliInfo); err != nil {
		return nil, err
	}

	return &gapi.Deploy{
		Name: Name,
		Noop: c.Bool("noop"),
		Sema: c.Int("sema"),
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

// Mgmt
// Copyright (C) 2013-2018+ James Shubin and the project contributors
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

package yamlgraph

import (
	"fmt"
	"log"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"

	errwrap "github.com/pkg/errors"
	"github.com/urfave/cli"
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

	data        gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(c *cli.Context, fs resources.Fs) (*gapi.Deploy, error) {
	if s := c.String(Name); c.IsSet(Name) {
		if s == "" {
			return nil, fmt.Errorf("input yaml is empty")
		}

		// single file input only
		if err := gapi.CopyFileToFs(fs, s, Start); err != nil {
			return nil, errwrap.Wrapf(err, "can't copy yaml from `%s` to `%s`", s, Start)
		}

		return &gapi.Deploy{
			Name: Name,
			Noop: c.GlobalBool("noop"),
			Sema: c.GlobalInt("sema"),
			GAPI: &GAPI{
				InputURI: fs.URI(),
				// TODO: add properties here...
			},
		}, nil
	}
	return nil, nil // we weren't activated!
}

// CliFlags returns a list of flags used by this deploy subcommand.
func (obj *GAPI) CliFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  Name,
			Value: "",
			Usage: "yaml graph definition to run",
		},
	}
}

// Init initializes the yamlgraph GAPI struct.
func (obj *GAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.InputURI == "" {
		return fmt.Errorf("the InputURI param must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	return nil
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
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

	config := ParseConfigFromFile(b)
	if config == nil {
		return nil, fmt.Errorf("%s: ParseConfigFromFile returned nil", Name)
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
				Err:  fmt.Errorf("%s: GAPI is not initialized", Name),
				Exit: true, // exit, b/c programming error?
			}
			ch <- next
			return
		}
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		var watchChan chan error
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
			case <-obj.closeChan:
				return
			}

			log.Printf("%s: Generating new graph...", Name)
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
		return fmt.Errorf("%s: GAPI is not initialized", Name)
	}
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

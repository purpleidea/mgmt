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

package hcl

import (
	"fmt"
	"log"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/recwatch"
	"github.com/purpleidea/mgmt/resources"

	errwrap "github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// Name is the name of this frontend.
	Name = "hcl"
	// Start is the entry point filename that we use. It is arbitrary.
	Start = "/start.hcl"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI ...
type GAPI struct {
	InputURI string

	initialized   bool
	data          gapi.Data
	wg            sync.WaitGroup
	closeChan     chan struct{}
	configWatcher *recwatch.ConfigWatcher
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(c *cli.Context, fs resources.Fs) (*gapi.Deploy, error) {
	if s := c.String(Name); c.IsSet(Name) {
		if s == "" {
			return nil, fmt.Errorf("%s input is empty", Name)
		}

		// TODO: single file input for now
		if err := gapi.CopyFileToFs(fs, s, Start); err != nil {
			return nil, errwrap.Wrapf(err, "can't copy code from `%s` to `%s`", s, Start)
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
			Name:  fmt.Sprintf("%s", Name),
			Value: "",
			Usage: "hcl graph definition to run",
		},
	}
}

// Init ...
func (obj *GAPI) Init(d gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.InputURI == "" {
		return fmt.Errorf("the InputURI param must be specified")
	}
	obj.data = d
	obj.closeChan = make(chan struct{})
	obj.initialized = true
	obj.configWatcher = recwatch.NewConfigWatcher()

	return nil
}

// Graph ...
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	fs, err := obj.data.World.Fs(obj.InputURI) // open the remote file system
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't load code from file system `%s`", obj.InputURI)
	}

	b, err := fs.ReadFile(Start) // read the single file out of it
	if err != nil {
		return nil, errwrap.Wrapf(err, "can't read code from file `%s`", Start)
	}

	config, err := loadHcl(b)
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
				Err:  fmt.Errorf("%s: GAPI is not initialized", Name),
				Exit: true,
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
			case <-startChan:
				startChan = nil
			case err, ok = <-watchChan:
				if !ok {
					return
				}
			case <-obj.closeChan:
				return
			}

			log.Printf("%s: generating new graph", Name)
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
		return fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	obj.configWatcher.Close()
	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false
	return nil
}

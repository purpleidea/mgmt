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

package lang

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/resources"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// Name is the name of this frontend.
	Name = "lang"
	// Start is the entry point filename that we use. It is arbitrary.
	Start = "/start." + FileNameExtension // FIXME: replace with a proper code entry point schema (directory schema)
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main lang GAPI interface.
type GAPI struct {
	InputURI string // input URI of code file system to run

	lang *Lang // lang struct

	data        gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          *sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(c *cli.Context, fs resources.Fs) (*gapi.Deploy, error) {
	if s := c.String(Name); c.IsSet(Name) {
		if s == "" {
			return nil, fmt.Errorf("input code is empty")
		}

		// read through this local path, and store it in our file system
		// since our deploy should work anywhere in the cluster, let the
		// engine ensure that this file system is replicated everywhere!

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
			Name:  fmt.Sprintf("%s, %s", Name, Name[0:1]),
			Value: "",
			Usage: "language code path to deploy",
		},
	}
}

// Init initializes the lang GAPI struct.
func (obj *GAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.InputURI == "" {
		return fmt.Errorf("the InputURI param must be specified")
	}
	obj.data = data // store for later
	obj.closeChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}
	obj.initialized = true
	return nil
}

// LangInit is a wrapper around the lang Init method.
func (obj *GAPI) LangInit() error {
	if obj.lang != nil {
		return nil // already ran init, close first!
	}

	fs, err := obj.data.World.Fs(obj.InputURI) // open the remote file system
	if err != nil {
		return errwrap.Wrapf(err, "can't load code from file system `%s`", obj.InputURI)
	}

	b, err := fs.ReadFile(Start) // read the single file out of it
	if err != nil {
		return errwrap.Wrapf(err, "can't read code from file `%s`", Start)
	}

	code := strings.NewReader(string(b))
	obj.lang = &Lang{
		Input:    code, // string as an interface that satisfies io.Reader
		Hostname: obj.data.Hostname,
		World:    obj.data.World,
		Debug:    obj.data.Debug,
	}
	if err := obj.lang.Init(); err != nil {
		return errwrap.Wrapf(err, "can't init the lang")
	}
	return nil
}

// LangClose is a wrapper around the lang Close method.
func (obj *GAPI) LangClose() error {
	if obj.lang != nil {
		err := obj.lang.Close()
		obj.lang = nil                                    // clear it to avoid double closing
		return errwrap.Wrapf(err, "can't close the lang") // nil passthrough
	}
	return nil
}

// Graph returns a current Graph.
func (obj *GAPI) Graph() (*pgraph.Graph, error) {
	if !obj.initialized {
		return nil, fmt.Errorf("%s: GAPI is not initialized", Name)
	}

	g, err := obj.lang.Interpret()
	if err != nil {
		return nil, errwrap.Wrapf(err, "%s: interpret error", Name)
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
		startChan := make(chan struct{}) // start signal
		close(startChan)                 // kick it off!

		streamChan := make(chan error)
		//defer obj.LangClose() // close any old lang

		var ok bool
		for {
			var err error
			var langSwap bool // do we need to swap the lang object?
			select {
			// TODO: this should happen in ConfigWatch instead :)
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				err = nil       // set nil as the message to send
				langSwap = true

			case err, ok = <-streamChan: // a variable changed
				if !ok { // the channel closed!
					return
				}

			case <-obj.closeChan:
				return
			}
			log.Printf("%s: Generating new graph...", Name)

			// skip this to pass through the err if present
			if langSwap && err == nil {
				log.Printf("%s: Swap!", Name)
				// run up to these three but fail on err
				if e := obj.LangClose(); e != nil { // close any old lang
					err = e // pass through the err
				} else if e := obj.LangInit(); e != nil { // init the new one!
					err = e // pass through the err

					// Always run LangClose after LangInit
					// when done. This is currently needed
					// because we should tell the lang obj
					// to shut down all the running facts.
					if e := obj.LangClose(); e != nil {
						err = multierr.Append(err, e) // list of errors
					}
				} else {

					if obj.data.NoStreamWatch { // TODO: do we want to allow this for the lang?
						streamChan = nil
					} else {
						// stream for lang events
						streamChan = obj.lang.Stream() // update stream
					}
					continue // wait for stream to trigger
				}
			}

			next := gapi.Next{
				Exit: err != nil, // TODO: do we want to shutdown?
				Err:  err,
			}
			select {
			case ch <- next: // trigger a run (send a msg)
				if err != nil {
					return
				}
			// unblock if we exit while waiting to send!
			case <-obj.closeChan:
				return
			}
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
	obj.LangClose()         // close lang, esp. if blocked in Stream() wait
	obj.initialized = false // closed = true
	return nil
}

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

package puppet

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	errwrap "github.com/pkg/errors"
	"github.com/urfave/cli"
)

const (
	// Name is the name of this frontend.
	Name = "puppet"
	// PuppetFile is the entry point filename that we use. It is arbitrary.
	PuppetFile = "/file.pp"
	// PuppetConf is the entry point config filename that we use.
	PuppetConf = "/puppet.conf"
	// PuppetSite is the entry point folder that we use.
	PuppetSite = "/puppet/"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main puppet GAPI interface.
type GAPI struct {
	InputURI string
	Mode     string // agent, file, string, dir

	puppetFile   string
	puppetString string
	puppetDir    string
	puppetConf   string // the path to an alternate puppet.conf file
	data         gapi.Data
	initialized  bool
	closeChan    chan struct{}
	wg           sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error.
func (obj *GAPI) Cli(c *cli.Context, fs engine.Fs) (*gapi.Deploy, error) {
	if s := c.String(Name); c.IsSet(Name) {
		if s == "" {
			return nil, fmt.Errorf("%s input is empty", Name)
		}

		isDir := func(p string) (bool, error) {
			if !strings.HasPrefix(p, "/") {
				return false, nil
			}
			if !strings.HasSuffix(s, "/") {
				return false, nil
			}
			fi, err := os.Stat(p)
			if err != nil {
				return false, err
			}
			return fi.IsDir(), nil
		}

		var mode string
		if s == "agent" {
			mode = "agent"

		} else if strings.HasSuffix(s, ".pp") {
			mode = "file"
			if err := gapi.CopyFileToFs(fs, s, PuppetFile); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy code from `%s` to `%s`", s, PuppetFile)
			}

		} else if exists, err := isDir(s); err != nil {
			return nil, errwrap.Wrapf(err, "can't read dir `%s`", s)

		} else if err == nil && exists { // from the isDir result...
			// we have a whole directory of files to run
			mode = "dir"
			// TODO: this code path is untested! test and then rm this notice
			if err := gapi.CopyDirToFs(fs, s, PuppetSite); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy code to `%s`", PuppetSite)
			}

		} else {
			mode = "string"
			if err := gapi.CopyStringToFs(fs, s, PuppetFile); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy code to `%s`", PuppetFile)
			}
		}

		// TODO: do we want to include this if we have mode == "dir" ?
		if pc := c.String("puppet-conf"); c.IsSet("puppet-conf") {
			if err := gapi.CopyFileToFs(fs, pc, PuppetConf); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy puppet conf from `%s`")
			}
		}

		return &gapi.Deploy{
			Name: Name,
			Noop: c.GlobalBool("noop"),
			Sema: c.GlobalInt("sema"),
			GAPI: &GAPI{
				InputURI: fs.URI(),
				Mode:     mode,
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
			Name:  "puppet, p",
			Value: "",
			Usage: "load graph from puppet, optionally takes a manifest or path to manifest file",
		},
		cli.StringFlag{
			Name:  "puppet-conf",
			Value: "",
			Usage: "the path to an alternate puppet.conf file",
		},
	}
}

// Init initializes the puppet GAPI struct.
func (obj *GAPI) Init(data gapi.Data) error {
	if obj.initialized {
		return fmt.Errorf("already initialized")
	}
	if obj.InputURI == "" {
		return fmt.Errorf("the InputURI param must be specified")
	}
	switch obj.Mode {
	case "agent", "file", "string", "dir":
		// pass
	default:
		return fmt.Errorf("the Mode param is invalid")
	}
	obj.data = data // store for later

	fs, err := obj.data.World.Fs(obj.InputURI) // open the remote file system
	if err != nil {
		return errwrap.Wrapf(err, "can't load data from file system `%s`", obj.InputURI)
	}

	if obj.Mode == "file" {
		b, err := fs.ReadFile(PuppetFile) // read the single file out of it
		if err != nil {
			return errwrap.Wrapf(err, "can't read code from file `%s`", PuppetFile)
		}

		// store the puppet file on disk for other binaries to see and use
		prefix := fmt.Sprintf("%s-%s-%s", data.Program, data.Hostname, strings.Replace(PuppetFile, "/", "", -1))
		tmpfile, err := ioutil.TempFile("", prefix)
		if err != nil {
			return errwrap.Wrapf(err, "can't create temp file")
		}
		obj.puppetFile = tmpfile.Name() // path to temp file
		defer tmpfile.Close()
		if _, err := tmpfile.Write(b); err != nil {
			return errwrap.Wrapf(err, "can't write file")
		}

	} else if obj.Mode == "string" {
		b, err := fs.ReadFile(PuppetFile) // read the single code string out of it
		if err != nil {
			return errwrap.Wrapf(err, "can't read code from file `%s`", PuppetFile)
		}
		obj.puppetString = string(b)

	} else if obj.Mode == "dir" {
		// store the puppet files on disk for other binaries to see and use
		prefix := fmt.Sprintf("%s-%s-%s", data.Program, data.Hostname, strings.Replace(PuppetSite, "/", "", -1))
		tmpdirName, err := ioutil.TempDir("", prefix)
		if err != nil {
			return errwrap.Wrapf(err, "can't create temp dir")
		}
		if tmpdirName == "" || tmpdirName == "/" {
			return fmt.Errorf("bad tmpdir created")
		}
		obj.puppetDir = tmpdirName // path to temp dir
		// TODO: this code path is untested! test and then rm this notice
		if err := util.CopyFsToDisk(fs, PuppetSite, tmpdirName, false); err != nil {
			return errwrap.Wrapf(err, "can't copy dir")
		}
	}

	if fi, err := fs.Stat(PuppetConf); err == nil && !fi.IsDir() { // if exists?
		b, err := fs.ReadFile(PuppetConf) // read the single file out of it
		if err != nil {
			return errwrap.Wrapf(err, "can't read config from file `%s`", PuppetConf)
		}

		// store the puppet conf on disk for other binaries to see and use
		prefix := fmt.Sprintf("%s-%s-%s", data.Program, data.Hostname, strings.Replace(PuppetConf, "/", "", -1))
		tmpfile, err := ioutil.TempFile("", prefix)
		if err != nil {
			return errwrap.Wrapf(err, "can't create temp file")
		}
		obj.puppetConf = tmpfile.Name() // path to temp file
		defer tmpfile.Close()
		if _, err := tmpfile.Write(b); err != nil {
			return errwrap.Wrapf(err, "can't write file")
		}
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
	config, err := obj.ParseConfigFromPuppet()
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, fmt.Errorf("function ParseConfigFromPuppet returned nil")
	}
	g, err := config.NewGraphFromConfig(obj.data.Hostname, obj.data.World, obj.data.Noop)
	return g, err
}

// Next returns nil errors every time there could be a new graph.
func (obj *GAPI) Next() chan gapi.Next {
	puppetChan := func() <-chan time.Time { // helper function
		return time.Tick(time.Duration(obj.refreshInterval()) * time.Second)
	}
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

		var pChan <-chan time.Time
		// NOTE: we don't look at obj.data.NoConfigWatch since emulating
		// puppet means we do not switch graphs on code changes anyways.
		if obj.data.NoStreamWatch {
			pChan = nil
		} else {
			pChan = puppetChan()
		}

		for {
			select {
			case <-startChan: // kick the loop once at start
				startChan = nil // disable
				// pass
			case _, ok := <-pChan:
				if !ok { // the channel closed!
					return
				}
			case <-obj.closeChan:
				return
			}

			obj.data.Logf("generating new graph...")
			if obj.data.NoStreamWatch {
				pChan = nil
			} else {
				pChan = puppetChan() // TODO: okay to update interval in case it changed?
			}
			next := gapi.Next{
				//Exit: true, // TODO: for permanent shutdown!
				Err: nil,
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

	if obj.puppetFile != "" {
		os.Remove(obj.puppetFile) // clean up, don't bother with error
	}
	// make this as safe as possible, check we're removing a tempdir too!
	if obj.puppetDir != "" && obj.puppetDir != "/" && strings.HasPrefix(obj.puppetDir, os.TempDir()) {
		os.RemoveAll(obj.puppetDir)
	}
	obj.puppetString = "" // free!
	if obj.puppetConf != "" {
		os.Remove(obj.puppetConf)
	}

	close(obj.closeChan)
	obj.wg.Wait()
	obj.initialized = false // closed = true
	return nil
}

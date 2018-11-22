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
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	multierr "github.com/hashicorp/go-multierror"
	errwrap "github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/urfave/cli"
)

const (
	// Name is the name of this frontend.
	Name = "lang"

	// flagModulePath is the name of the module-path flag.
	flagModulePath = "module-path"

	// flagDownload is the name of the download flag.
	flagDownload = "download"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main lang GAPI interface.
type GAPI struct {
	InputURI string // input URI of code file system to run

	lang *Lang // lang struct

	// this data struct is only available *after* Init, so as a result, it
	// can not be used inside the Cli(...) method.
	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          *sync.WaitGroup // sync group for tunnel go routines
}

// CliFlags returns a list of flags used by the specified subcommand.
func (obj *GAPI) CliFlags(command string) []cli.Flag {
	result := []cli.Flag{}
	modulePath := cli.StringFlag{
		Name:   flagModulePath,
		Value:  "", // empty by default
		Usage:  "choose the modules path (absolute)",
		EnvVar: "MGMT_MODULE_PATH",
	}

	// add this only to run (not needed for get or deploy)
	if command == gapi.CommandRun {
		runFlags := []cli.Flag{
			cli.BoolFlag{
				Name:  flagDownload,
				Usage: "download any missing imports (as the get command does)",
			},
			cli.BoolFlag{
				Name:  "update",
				Usage: "update all dependencies to the latest versions",
			},
		}
		result = append(result, runFlags...)
	}

	switch command {
	case gapi.CommandGet:
		flags := []cli.Flag{
			cli.IntFlag{
				Name:  "depth d",
				Value: -1,
				Usage: "max recursion depth limit (-1 is unlimited)",
			},
			cli.IntFlag{
				Name:  "retry r",
				Value: 0, // any error is a failure by default
				Usage: "max number of retries (-1 is unlimited)",
			},
			//modulePath, // already defined below in fallthrough
		}
		result = append(result, flags...)
		fallthrough // at the moment, we want the same code input arg...
	case gapi.CommandRun:
		fallthrough
	case gapi.CommandDeploy:
		flags := []cli.Flag{
			cli.StringFlag{
				Name:  fmt.Sprintf("%s, %s", Name, Name[0:1]),
				Value: "",
				Usage: "code to deploy",
			},
			// TODO: removed (temporarily?)
			//cli.BoolFlag{
			//	Name:  "stdin",
			//	Usage: "use passthrough stdin",
			//},
			modulePath,
		}
		result = append(result, flags...)
	default:
		return []cli.Flag{}
	}

	return result
}

// Cli takes a cli.Context, and returns our GAPI if activated. All arguments
// should take the prefix of the registered name. On activation, if there are
// any validation problems, you should return an error. If this was not
// activated, then you should return a nil GAPI and a nil error. This is passed
// in a functional file system interface. For standalone usage, this will be a
// temporary memory-backed filesystem so that the same deploy API is used, and
// for normal clustered usage, this will be the normal implementation which is
// usually an etcd backed fs. At this point we should be copying the necessary
// local file system data into our fs for future use when the GAPI is running.
// IOW, running this Cli function, when activated, produces a deploy object
// which is run by our main loop. The difference between running from `deploy`
// or from `run` (both of which can activate this GAPI) is that `deploy` copies
// to an etcdFs, and `run` copies to a memFs. All GAPI's run off of the fs that
// is passed in.
func (obj *GAPI) Cli(cliInfo *gapi.CliInfo) (*gapi.Deploy, error) {
	c := cliInfo.CliContext
	cliContext := c.Parent()
	if cliContext == nil {
		return nil, fmt.Errorf("could not get cli context")
	}
	fs := cliInfo.Fs // copy files from local filesystem *into* this fs...
	prefix := ""     // TODO: do we need this?
	debug := cliInfo.Debug
	logf := func(format string, v ...interface{}) {
		cliInfo.Logf(Name+": "+format, v...)
	}

	if !c.IsSet(Name) {
		return nil, nil // we weren't activated!
	}

	// empty by default (don't set for deploy, only download)
	modules := c.String(flagModulePath)
	if modules != "" && (!strings.HasPrefix(modules, "/") || !strings.HasSuffix(modules, "/")) {
		return nil, fmt.Errorf("module path is not an absolute directory")
	}

	// TODO: while reading through trees of metadata files, we could also
	// check the license compatibility of deps...

	osFs := afero.NewOsFs()
	readOnlyOsFs := afero.NewReadOnlyFs(osFs) // can't be readonly to dl!
	//bp := afero.NewBasePathFs(osFs, base) // TODO: can this prevent parent dir access?
	afs := &afero.Afero{Fs: readOnlyOsFs} // wrap so that we're implementing ioutil
	localFs := &util.Fs{Afero: afs}       // always the local fs
	downloadAfs := &afero.Afero{Fs: osFs}
	downloadFs := &util.Fs{Afero: downloadAfs} // TODO: use with a parent path preventer?

	// the fs input here is the local fs we're reading to get the files from
	// this is different from the fs variable which is our output dest!!!
	output, err := parseInput(c.String(Name), localFs)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not activate an input parser")
	}

	// no need to run recursion detection since this is the beginning
	// TODO: do the paths need to be cleaned for "../" before comparison?

	logf("lexing/parsing...")
	ast, err := LexParse(bytes.NewReader(output.Main))
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not generate AST")
	}
	if debug {
		logf("behold, the AST: %+v", ast)
	}

	var downloader interfaces.Downloader
	if c.IsSet(flagDownload) && c.Bool(flagDownload) {
		downloadInfo := &interfaces.DownloadInfo{
			Fs: downloadFs, // the local fs!

			// flags are passed in during Init()
			Noop:   cliContext.Bool("noop"),
			Sema:   cliContext.Int("sema"),
			Update: c.Bool("update"),

			Debug: debug,
			Logf: func(format string, v ...interface{}) {
				// TODO: is this a sane prefix to use here?
				logf("get: "+format, v...)
			},
		}
		// this fulfills the interfaces.Downloader interface
		downloader = &Downloader{
			Depth: c.Int("depth"), // default of infinite is -1
			Retry: c.Int("retry"), // infinite is -1
		}
		if err := downloader.Init(downloadInfo); err != nil {
			return nil, errwrap.Wrapf(err, "could not initialize downloader")
		}
	}

	importGraph, err := pgraph.NewGraph("importGraph")
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not create graph")
	}
	importVertex := &pgraph.SelfVertex{
		Name:  "",          // first node is the empty string
		Graph: importGraph, // store a reference to ourself
	}
	importGraph.AddVertex(importVertex)

	logf("init...")
	// init and validate the structure of the AST
	data := &interfaces.Data{
		Fs:         localFs,     // the local fs!
		Base:       output.Base, // base dir (absolute path) that this is rooted in
		Files:      output.Files,
		Imports:    importVertex,
		Metadata:   output.Metadata,
		Modules:    modules,
		Downloader: downloader,

		//World: obj.World, // TODO: do we need this?
		Prefix: prefix,
		Debug:  debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			logf("ast: "+format, v...)
		},
	}
	// some of this might happen *after* interpolate in SetScope or Unify...
	if err := ast.Init(data); err != nil {
		return nil, errwrap.Wrapf(err, "could not init and validate AST")
	}

	logf("interpolating...")
	// interpolate strings and other expansionable nodes in AST
	interpolated, err := ast.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate AST")
	}

	// top-level, built-in, initial global scope
	scope := &interfaces.Scope{
		Variables: map[string]interfaces.Expr{
			"purpleidea": &ExprStr{V: "hello world!"}, // james says hi
			// TODO: change to a func when we can change hostname dynamically!
			"hostname": &ExprStr{V: ""}, // NOTE: empty b/c not used
		},
		// all the built-in top-level, core functions enter here...
		Functions: funcs.LookupPrefix(""),
	}

	logf("building scope...")
	// propagate the scope down through the AST...
	// We use SetScope because it follows all of the imports through. I did
	// not think we needed to pass in an initial scope because the download
	// operation should not depend on any initial scope values, since those
	// would all be runtime changes, and we do not support dynamic imports,
	// however, we need to since we're doing type unification to err early!
	if err := interpolated.SetScope(scope); err != nil { // empty initial scope!
		return nil, errwrap.Wrapf(err, "could not set scope")
	}

	// apply type unification
	unificationLogf := func(format string, v ...interface{}) {
		if debug { // unification only has debug messages...
			logf("unification: "+format, v...)
		}
	}
	logf("running type unification...")
	if err := unification.Unify(interpolated, unification.SimpleInvariantSolverLogger(unificationLogf)); err != nil {
		return nil, errwrap.Wrapf(err, "could not unify types")
	}

	// get the list of needed files (this is available after SetScope)
	fileList, err := CollectFiles(interpolated)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not collect files")
	}

	// add in our initial files

	// we can sometimes be missing our top-level metadata.yaml and main.mcl
	files := []string{}
	files = append(files, output.Files...)
	files = append(files, fileList...)

	// run some copy operations to add data into the filesystem
	for _, fn := range output.Workers {
		if err := fn(fs); err != nil {
			return nil, err
		}
	}

	// TODO: do we still need this, now that we have the Imports DAG?
	noDuplicates := util.StrRemoveDuplicatesInList(files)
	if len(noDuplicates) != len(files) {
		// programming error here or in this logical test
		return nil, fmt.Errorf("duplicates in file list found")
	}

	// sort by depth dependency order! (or mkdir -p all the dirs first)
	// TODO: is this natively already in a correctly sorted order?
	util.PathSlice(files).Sort() // sort it
	for _, src := range files {  // absolute paths
		// rebase path src to root file system of "/" for etcdfs...
		dst, err := util.Rebase(src, output.Base, "/")
		if err != nil {
			// possible programming error
			return nil, errwrap.Wrapf(err, "malformed source file path: `%s`", src)
		}

		if strings.HasSuffix(src, "/") { // it's a dir
			// TODO: add more tests to this (it is actually CopyFs)
			if err := gapi.CopyDirToFs(fs, src, dst); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy dir from `%s` to `%s`", src, dst)
			}
			continue
		}
		// it's a regular file path
		if err := gapi.CopyFileToFs(fs, src, dst); err != nil {
			return nil, errwrap.Wrapf(err, "can't copy file from `%s` to `%s`", src, dst)
		}
	}

	// display the deploy fs tree
	if debug || true { // TODO: should this only be shown on debug?
		logf("input: %s", c.String(Name))
		tree, err := util.FsTree(fs, "/")
		if err != nil {
			return nil, err
		}
		logf("tree:\n%s", tree)
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

// Init initializes the lang GAPI struct.
func (obj *GAPI) Init(data *gapi.Data) error {
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
	if obj.InputURI == "-" {
		return fmt.Errorf("stdin passthrough is not supported at this time")
	}

	fs, err := obj.data.World.Fs(obj.InputURI) // open the remote file system
	if err != nil {
		return errwrap.Wrapf(err, "can't load code from file system `%s`", obj.InputURI)
	}
	// the lang always tries to load from this standard path: /metadata.yaml
	input := "/" + interfaces.MetadataFilename // path in remote fs

	obj.lang = &Lang{
		Fs:    fs,
		Input: input,

		Hostname: obj.data.Hostname,
		World:    obj.data.World,
		Debug:    obj.data.Debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: add the Name prefix in parent logger
			obj.data.Logf(Name+": "+format, v...)
		},
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
			obj.data.Logf("generating new graph...")

			// skip this to pass through the err if present
			if langSwap && err == nil {
				obj.data.Logf("swap!")
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
						obj.data.Logf("warning: language will not stream")
						// send only one event
						limitChan := make(chan error)
						obj.wg.Add(1)
						go func() {
							defer obj.wg.Done()
							defer close(limitChan)
							select {
							// only one
							case err, ok := <-obj.lang.Stream():
								if !ok {
									return
								}
								select {
								case limitChan <- err:
								case <-obj.closeChan:
									return
								}
							case <-obj.closeChan:
								return
							}
						}()
						streamChan = limitChan
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

// Get runs the necessary downloads. This basically runs the lexer, parser and
// sets the scope so that all the imports are followed. It passes a downloader
// in, which can be used to pull down or update any missing imports. This will
// also work when called with the download flag during a normal execution run.
func (obj *GAPI) Get(getInfo *gapi.GetInfo) error {
	c := getInfo.CliContext
	cliContext := c.Parent()
	if cliContext == nil {
		return fmt.Errorf("could not get cli context")
	}
	prefix := "" // TODO: do we need this?
	debug := getInfo.Debug
	logf := getInfo.Logf

	// empty by default (don't set for deploy, only download)
	modules := c.String(flagModulePath)
	if modules != "" && (!strings.HasPrefix(modules, "/") || !strings.HasSuffix(modules, "/")) {
		return fmt.Errorf("module path is not an absolute directory")
	}

	osFs := afero.NewOsFs()
	readOnlyOsFs := afero.NewReadOnlyFs(osFs) // can't be readonly to dl!
	//bp := afero.NewBasePathFs(osFs, base) // TODO: can this prevent parent dir access?
	afs := &afero.Afero{Fs: readOnlyOsFs} // wrap so that we're implementing ioutil
	localFs := &util.Fs{Afero: afs}       // always the local fs
	downloadAfs := &afero.Afero{Fs: osFs}
	downloadFs := &util.Fs{Afero: downloadAfs} // TODO: use with a parent path preventer?

	// the fs input here is the local fs we're reading to get the files from
	// this is different from the fs variable which is our output dest!!!
	output, err := parseInput(c.String(Name), localFs)
	if err != nil {
		return errwrap.Wrapf(err, "could not activate an input parser")
	}

	// no need to run recursion detection since this is the beginning
	// TODO: do the paths need to be cleaned for "../" before comparison?

	logf("lexing/parsing...")
	ast, err := LexParse(bytes.NewReader(output.Main))
	if err != nil {
		return errwrap.Wrapf(err, "could not generate AST")
	}
	if debug {
		logf("behold, the AST: %+v", ast)
	}

	downloadInfo := &interfaces.DownloadInfo{
		Fs: downloadFs, // the local fs!

		// flags are passed in during Init()
		Noop:   cliContext.Bool("noop"),
		Sema:   cliContext.Int("sema"),
		Update: cliContext.Bool("update"),

		Debug: debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			logf("get: "+format, v...)
		},
	}
	// this fulfills the interfaces.Downloader interface
	downloader := &Downloader{
		Depth: c.Int("depth"), // default of infinite is -1
		Retry: c.Int("retry"), // infinite is -1
	}
	if err := downloader.Init(downloadInfo); err != nil {
		return errwrap.Wrapf(err, "could not initialize downloader")
	}

	importGraph, err := pgraph.NewGraph("importGraph")
	if err != nil {
		return errwrap.Wrapf(err, "could not create graph")
	}
	importVertex := &pgraph.SelfVertex{
		Name:  "",          // first node is the empty string
		Graph: importGraph, // store a reference to ourself
	}
	importGraph.AddVertex(importVertex)

	logf("init...")
	// init and validate the structure of the AST
	data := &interfaces.Data{
		Fs:         localFs,     // the local fs!
		Base:       output.Base, // base dir (absolute path) that this is rooted in
		Files:      output.Files,
		Imports:    importVertex,
		Metadata:   output.Metadata,
		Modules:    modules,
		Downloader: downloader,

		//World: obj.World, // TODO: do we need this?
		Prefix: prefix,
		Debug:  debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			logf("ast: "+format, v...)
		},
	}
	// some of this might happen *after* interpolate in SetScope or Unify...
	if err := ast.Init(data); err != nil {
		return errwrap.Wrapf(err, "could not init and validate AST")
	}

	logf("interpolating...")
	// interpolate strings and other expansionable nodes in AST
	interpolated, err := ast.Interpolate()
	if err != nil {
		return errwrap.Wrapf(err, "could not interpolate AST")
	}

	logf("building scope...")
	// propagate the scope down through the AST...
	// we use SetScope because it follows all of the imports through. i
	// don't think we need to pass in an initial scope because the download
	// operation shouldn't depend on any initial scope values, since those
	// would all be runtime changes, and we do not support dynamic imports!
	if err := interpolated.SetScope(nil); err != nil { // empty initial scope!
		return errwrap.Wrapf(err, "could not set scope")
	}

	return nil // success!
}

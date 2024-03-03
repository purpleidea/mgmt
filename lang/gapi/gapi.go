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
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package gapi is the Graph API implementation for the mcl language frontend.
package gapi

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/gapi"
	"github.com/purpleidea/mgmt/lang"
	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/download"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/interpolate"
	"github.com/purpleidea/mgmt/lang/parser"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"

	"github.com/spf13/afero"
)

const (
	// Name is the name of this frontend.
	Name = "lang"
)

func init() {
	gapi.Register(Name, func() gapi.GAPI { return &GAPI{} }) // register
}

// GAPI implements the main lang GAPI interface.
type GAPI struct {
	InputURI string // input URI of code file system to run

	lang   *lang.Lang // lang struct
	wgRun  *sync.WaitGroup
	ctx    context.Context
	cancel func()
	reterr error

	// this data struct is only available *after* Init, so as a result, it
	// can not be used inside the Cli(...) method.
	data        *gapi.Data
	initialized bool
	closeChan   chan struct{}
	wg          *sync.WaitGroup // sync group for tunnel go routines
}

// Cli takes an *Info struct, and returns our deploy if activated, and if there
// are any validation problems, you should return an error. If there is no
// deploy, then you should return a nil deploy and a nil error. This is passed
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
func (obj *GAPI) Cli(info *gapi.Info) (*gapi.Deploy, error) {
	args, ok := info.Args.(*cliUtil.LangArgs)
	if !ok {
		// programming error
		return nil, fmt.Errorf("could not convert to our struct")
	}

	fs := info.Fs // copy files from local filesystem *into* this fs...
	prefix := ""  // TODO: do we need this?
	debug := info.Debug
	logf := func(format string, v ...interface{}) {
		info.Logf(Name+": "+format, v...)
	}

	// empty by default (don't set for deploy, only download)
	modules := args.ModulePath
	if modules != "" && (!strings.HasPrefix(modules, "/") || !strings.HasSuffix(modules, "/")) {
		return nil, fmt.Errorf("module path is not an absolute directory")
	}

	// TODO: while reading through trees of metadata files, we could also
	// check the license compatibility of deps...

	osFs := afero.NewOsFs()
	readOnlyOsFs := afero.NewReadOnlyFs(osFs) // can't be readonly to dl!
	//bp := afero.NewBasePathFs(osFs, base) // TODO: can this prevent parent dir access?
	afs := &afero.Afero{Fs: readOnlyOsFs} // wrap so that we're implementing ioutil
	localFs := &util.AferoFs{Afero: afs}  // always the local fs
	downloadAfs := &afero.Afero{Fs: osFs}
	downloadFs := &util.AferoFs{Afero: downloadAfs} // TODO: use with a parent path preventer?

	// the fs input here is the local fs we're reading to get the files from
	// this is different from the fs variable which is our output dest!!!
	output, err := inputs.ParseInput(args.Input, localFs)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not activate an input parser")
	}

	// no need to run recursion detection since this is the beginning
	// TODO: do the paths need to be cleaned for "../" before comparison?

	logf("lexing/parsing...")
	xast, err := parser.LexParse(bytes.NewReader(output.Main))
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not generate AST")
	}
	if debug {
		logf("behold, the AST: %+v", xast)
	}

	// This runs the necessary downloads. It passes a downloader in, which
	// can be used to pull down or update any missing imports.
	var downloader interfaces.Downloader
	if args.Download {
		downloadInfo := &interfaces.DownloadInfo{
			Fs: downloadFs, // the local fs!

			// flags are passed in during Init()
			Noop:   info.Flags.Noop,
			Sema:   info.Flags.Sema,
			Update: args.Update,

			Debug: debug,
			Logf: func(format string, v ...interface{}) {
				// TODO: is this a sane prefix to use here?
				logf("get: "+format, v...)
			},
		}
		// this fulfills the interfaces.Downloader interface
		downloader = &download.Downloader{
			Depth: args.Depth, // default of infinite is -1
			Retry: args.Retry, // infinite is -1
		}
		if err := downloader.Init(downloadInfo); err != nil {
			return nil, errwrap.Wrapf(err, "could not initialize downloader")
		}
	}

	importGraph, err := pgraph.NewGraph("importGraph")
	if err != nil {
		return nil, err
	}
	importVertex := &pgraph.SelfVertex{
		Name:  "",          // first node is the empty string
		Graph: importGraph, // store a reference to ourself
	}
	importGraph.AddVertex(importVertex)

	logf("init...")
	// init and validate the structure of the AST
	data := &interfaces.Data{
		// TODO: add missing fields here if/when needed
		Fs:       output.FS,       // formerly: localFs // the local fs!
		FsURI:    output.FS.URI(), // formerly: localFs.URI() // TODO: is this right?
		Base:     output.Base,     // base dir (absolute path) that this is rooted in
		Files:    output.Files,
		Imports:  importVertex,
		Metadata: output.Metadata,
		Modules:  modules,

		LexParser:       parser.LexParse,
		Downloader:      downloader,
		StrInterpolater: interpolate.StrInterpolate,
		//Local: obj.Local, // TODO: do we need this?
		//World: obj.World, // TODO: do we need this?

		Prefix: prefix,
		Debug:  debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			logf("ast: "+format, v...)
		},
	}
	// some of this might happen *after* interpolate in SetScope or Unify...
	if err := xast.Init(data); err != nil {
		return nil, errwrap.Wrapf(err, "could not init and validate AST")
	}

	logf("interpolating...")
	// interpolate strings and other expansionable nodes in AST
	iast, err := xast.Interpolate()
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpolate AST")
	}

	hostname := ""
	if h := info.Flags.Hostname; h != nil {
		hostname = *h // it's optional, since this value is not used...
	}
	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		// TODO: change to a func when we can change hostname dynamically!
		"hostname": &ast.ExprStr{V: hostname}, // NOTE: can be empty b/c not used
	}
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		return nil, errwrap.Wrapf(err, "couldn't merge in consts")
	}

	// top-level, built-in, initial global scope
	scope := &interfaces.Scope{
		Variables: variables,
		// all the built-in top-level, core functions enter here...
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}

	logf("building scope...")
	// propagate the scope down through the AST...
	// We use SetScope because it follows all of the imports through. I did
	// not think we needed to pass in an initial scope because the download
	// operation should not depend on any initial scope values, since those
	// would all be runtime changes, and we do not support dynamic imports,
	// however, we need to since we're doing type unification to err early!
	if err := iast.SetScope(scope); err != nil { // empty initial scope!
		return nil, errwrap.Wrapf(err, "could not set scope")
	}

	// Previously the `get` command would stop here.
	if args.OnlyDownload {
		return nil, nil // success!
	}

	if !args.SkipUnify {
		// apply type unification
		unificationLogf := func(format string, v ...interface{}) {
			if debug { // unification only has debug messages...
				logf("unification: "+format, v...)
			}
		}
		logf("running type unification...")
		startTime := time.Now()
		unifier := &unification.Unifier{
			AST:    iast,
			Solver: unification.SimpleInvariantSolverLogger(unificationLogf),
			Debug:  debug,
			Logf:   unificationLogf,
		}
		unifyErr := unifier.Unify()
		delta := time.Since(startTime)
		formatted := delta.String()
		if delta.Milliseconds() > 1000 { // 1 second
			formatted = delta.Truncate(time.Millisecond).String()
		}
		if unifyErr != nil {
			if args.OnlyUnify {
				logf("type unification failed after %s", formatted)
			}
			return nil, errwrap.Wrapf(unifyErr, "could not unify types")
		}

		if args.OnlyUnify {
			logf("type unification succeeded in %s", formatted)
			return nil, nil // we end early
		}
	}

	// get the list of needed files (this is available after SetScope)
	fileList, err := ast.CollectFiles(iast)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not collect files")
	}

	// add in our initial files

	// we can sometimes be missing our top-level metadata.yaml and main.mcl
	files := []string{}
	files = append(files, output.Files...)
	files = append(files, fileList...)

	writeableFS, ok := fs.(engine.WriteableFS)
	if !ok {
		return nil, fmt.Errorf("the FS was not writeable")
	}

	// run some copy operations to add data into the filesystem
	for _, fn := range output.Workers {
		if err := fn(writeableFS); err != nil {
			return nil, err
		}
	}

	// TODO: do we still need this, now that we have the Imports DAG?
	noDuplicates := util.StrRemoveDuplicatesInList(files)
	if len(noDuplicates) != len(files) {
		// programming error here or in this logical test
		return nil, fmt.Errorf("duplicates in file list found")
	}

	// Add any missing dirs, so that we don't need to use `MkdirAll`...
	// FIXME: It's possible that the dirs get generated upstream, but it's
	// not exactly clear where they'd need to get added into the list. If we
	// figure that out, we can remove this additional step. It's trickier,
	// because adding duplicates isn't desirable either.
	//dirs, err := util.MissingMkdirs(files)
	//if err != nil {
	//	// possible programming error
	//	return nil, errwrap.Wrapf(err, "unexpected missing mkdirs input")
	//}
	//parents := util.DirParents(output.Base)
	//parents = append(parents, output.Base) // include self
	//
	// And we don't want to include any of the parents above the Base dir...
	//for _, x := range dirs {
	//	if util.StrInList(x, parents) {
	//		continue
	//	}
	//	files = append(files, x)
	//}

	// sort by depth dependency order! (or mkdir -p all the dirs first)
	// TODO: is this natively already in a correctly sorted order?
	util.PathSlice(files).Sort() // sort it
	for _, src := range files {  // absolute paths
		// rebase path src to root file system of "/" for etcdfs...

		// everywhere we expect absolute, but we should use relative :/
		//tree, err := util.FsTree(fs, "/")
		//if err != nil {
		//	return nil, err
		//}
		//logf("tree:\n%s", tree)

		dst, err := util.Rebase(src, output.Base, "/")
		if err != nil {
			// possible programming error
			return nil, errwrap.Wrapf(err, "malformed source file path: `%s`", src)
		}

		if strings.HasSuffix(src, "/") { // it's a dir
			// FIXME: I think fixing CopyDirToFs might be better...
			if dst != "/" { // XXX: hack, don't nest the copy badly!
				out, err := util.RemovePathSuffix(dst)
				if err != nil {
					// possible programming error
					return nil, errwrap.Wrapf(err, "malformed dst dir path: `%s`", dst)
				}
				dst = out
			}
			// TODO: add more tests to this (it is actually CopyFs)
			// TODO: Used to be: CopyDirToFs, but it had issues...
			if err := gapi.CopyDirToFsForceAll(fs, src, dst); err != nil {
				return nil, errwrap.Wrapf(err, "can't copy dir from `%s` to `%s`", src, dst)
			}
			continue
		}
		// it's a regular file path
		if err := gapi.CopyFileToFs(writeableFS, src, dst); err != nil {
			return nil, errwrap.Wrapf(err, "can't copy file from `%s` to `%s`", src, dst)
		}
	}

	// display the deploy fs tree
	if debug || true { // TODO: should this only be shown on debug?
		logf("input: %s", args.Input)
		tree, err := util.FsTree(fs, "/")
		if err != nil {
			return nil, err
		}
		logf("tree:\n%s", tree)
	}

	return &gapi.Deploy{
		Name: Name,
		Noop: info.Flags.Noop,
		Sema: info.Flags.Sema,
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

	obj.lang = &lang.Lang{
		Fs:    fs,
		FsURI: obj.InputURI,
		Input: input,

		Hostname: obj.data.Hostname,
		Local:    obj.data.Local,
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

	// XXX: I'm certain I've probably got a deadlock or race somewhere here
	// or in lib/main.go so we'll fix it with an API fixup and rewrite soon
	obj.wgRun = &sync.WaitGroup{}
	obj.ctx, obj.cancel = context.WithCancel(context.Background())
	obj.wgRun.Add(1)
	go func() {
		defer obj.wgRun.Done()
		obj.reterr = obj.lang.Run(obj.ctx)
	}()

	return nil
}

// LangClose is a wrapper around the lang Close method.
func (obj *GAPI) LangClose() error {
	if obj.lang != nil {
		obj.cancel()
		obj.wgRun.Wait()
		err := obj.lang.Cleanup()
		err = errwrap.Append(err, obj.reterr)             // from obj.lang.Run
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

		streamChan := make(<-chan error)
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
			// XXX: redo this old garbage code
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
						err = errwrap.Append(err, e) // list of errors
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

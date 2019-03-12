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

package lang // TODO: move this into a sub package of lang/$name?

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/lang/funcs"
	_ "github.com/purpleidea/mgmt/lang/funcs/core" // import so the funcs register
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// make these available internally without requiring the import
	operatorFuncName = funcs.OperatorFuncName
	historyFuncName  = funcs.HistoryFuncName
	containsFuncName = funcs.ContainsFuncName
)

// Lang is the main language lexer/parser object.
type Lang struct {
	Fs engine.Fs // connected fs where input dir or metadata exists
	// Input is a string which specifies what the lang should run. It can
	// accept values in several different forms. If is passed a single dash
	// (-), then it will use `os.Stdin`. If it is passed a single .mcl file,
	// then it will attempt to run that. If it is passed a directory path,
	// then it will attempt to run from there. Instead, if it is passed the
	// path to a metadata file, then it will attempt to parse that and run
	// from that specification. If none of those match, it will attempt to
	// run the raw string as mcl code.
	Input string

	Hostname string
	World    engine.World
	Prefix   string
	Debug    bool
	Logf     func(format string, v ...interface{})

	ast   interfaces.Stmt // store main prog AST here
	funcs *funcs.Engine   // function event engine

	loadedChan chan struct{} // loaded signal

	streamChan chan error // signals a new graph can be created or problem
	//streamBurst bool // should we try and be bursty with the stream events?

	closeChan chan struct{} // close signal
	wg        *sync.WaitGroup
}

// Init initializes the lang struct, and starts up the initial data sources.
// NOTE: The trick is that we need to get the list of funcs to watch AND start
// watching them, *before* we pull their values, that way we'll know if they
// changed from the values we wanted.
func (obj *Lang) Init() error {
	obj.loadedChan = make(chan struct{})
	obj.streamChan = make(chan error)
	obj.closeChan = make(chan struct{})
	obj.wg = &sync.WaitGroup{}

	once := &sync.Once{}
	loadedSignal := func() { close(obj.loadedChan) } // only run once!

	if obj.Debug {
		obj.Logf("input: %s", obj.Input)
		tree, err := util.FsTree(obj.Fs, "/") // should look like gapi
		if err != nil {
			return err
		}
		obj.Logf("run tree:\n%s", tree)
	}

	// we used to support stdin passthrough, but we we got rid of it for now
	// the fs input here is the local fs we're reading to get the files from
	// which is usually etcdFs.
	output, err := parseInput(obj.Input, obj.Fs)
	if err != nil {
		return errwrap.Wrapf(err, "could not activate an input parser")
	}
	if len(output.Workers) > 0 {
		// either programming error, or someone hacked in something here
		// by the time *this* parseInput runs, we should be standardized
		return fmt.Errorf("input contained file system workers")
	}
	reader := bytes.NewReader(output.Main)

	// no need to run recursion detection since this is the beginning
	// TODO: do the paths need to be cleaned for "../" before comparison?

	// run the lexer/parser and build an AST
	obj.Logf("lexing/parsing...")
	// this reads an io.Reader, which might be a stream of multiple files...
	ast, err := LexParse(reader)
	if err != nil {
		return errwrap.Wrapf(err, "could not generate AST")
	}
	if obj.Debug {
		obj.Logf("behold, the AST: %+v", ast)
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

	obj.Logf("init...")
	// init and validate the structure of the AST
	data := &interfaces.Data{
		Fs:       obj.Fs,
		Base:     output.Base, // base dir (absolute path) the metadata file is in
		Files:    output.Files,
		Imports:  importVertex,
		Metadata: output.Metadata,
		Modules:  "/" + interfaces.ModuleDirectory, // do not set from env for a deploy!

		//World: obj.World, // TODO: do we need this?
		Prefix: obj.Prefix,
		Debug:  obj.Debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			obj.Logf("ast: "+format, v...)
		},
	}
	// some of this might happen *after* interpolate in SetScope or Unify...
	if err := ast.Init(data); err != nil {
		return errwrap.Wrapf(err, "could not init and validate AST")
	}

	obj.Logf("interpolating...")
	// interpolate strings and other expansionable nodes in AST
	interpolated, err := ast.Interpolate()
	if err != nil {
		return errwrap.Wrapf(err, "could not interpolate AST")
	}
	obj.ast = interpolated

	// top-level, built-in, initial global scope
	scope := &interfaces.Scope{
		Variables: map[string]interfaces.Expr{
			"purpleidea": &ExprStr{V: "hello world!"}, // james says hi
			// TODO: change to a func when we can change hostname dynamically!
			"hostname": &ExprStr{V: obj.Hostname},
		},
		// all the built-in top-level, core functions enter here...
		Functions: funcs.LookupPrefix(""),
	}

	obj.Logf("building scope...")
	// propagate the scope down through the AST...
	if err := obj.ast.SetScope(scope); err != nil {
		return errwrap.Wrapf(err, "could not set scope")
	}

	// apply type unification
	logf := func(format string, v ...interface{}) {
		if obj.Debug { // unification only has debug messages...
			obj.Logf("unification: "+format, v...)
		}
	}
	obj.Logf("running type unification...")
	if err := unification.Unify(obj.ast, unification.SimpleInvariantSolverLogger(logf)); err != nil {
		return errwrap.Wrapf(err, "could not unify types")
	}

	obj.Logf("building function graph...")
	// we assume that for some given code, the list of funcs doesn't change
	// iow, we don't support variable, variables or absurd things like that
	graph, err := obj.ast.Graph() // build the graph of functions
	if err != nil {
		return errwrap.Wrapf(err, "could not generate function graph")
	}

	if obj.Debug {
		obj.Logf("function graph: %+v", graph)
		graph.Logf(obj.Logf) // log graph output with this logger...
	}

	if graph.NumVertices() == 0 { // no funcs to load!
		// send only one signal since we won't ever send after this!
		obj.Logf("static graph found")
		obj.wg.Add(1)
		go func() {
			defer obj.wg.Done()
			defer close(obj.streamChan) // no more events are coming!
			close(obj.loadedChan)       // signal
			select {
			case obj.streamChan <- nil: // send one signal
				// pass
			case <-obj.closeChan:
				return
			}
		}()
		return nil // exit early, no funcs to load!
	}

	obj.funcs = &funcs.Engine{
		Graph:    graph, // not the same as the output graph!
		Hostname: obj.Hostname,
		World:    obj.World,
		Debug:    obj.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.Logf("funcs: "+format, v...)
		},
		Glitch: false, // FIXME: verify this functionality is perfect!
	}

	obj.Logf("function engine initializing...")
	if err := obj.funcs.Init(); err != nil {
		return errwrap.Wrapf(err, "init error with func engine")
	}

	obj.Logf("function engine validating...")
	if err := obj.funcs.Validate(); err != nil {
		return errwrap.Wrapf(err, "validate error with func engine")
	}

	obj.Logf("function engine starting...")
	// On failure, we expect the caller to run Close() to shutdown all of
	// the currently initialized (and running) funcs... This is needed if
	// we successfully ran `Run` but isn't needed only for Init/Validate.
	if err := obj.funcs.Run(); err != nil {
		return errwrap.Wrapf(err, "run error with func engine")
	}

	// wait for some activity
	obj.Logf("stream...")
	stream := obj.funcs.Stream()
	obj.wg.Add(1)
	go func() {
		obj.Logf("loop...")
		defer obj.wg.Done()
		defer close(obj.streamChan) // no more events are coming!
		for {
			var err error
			var ok bool
			select {
			case err, ok = <-stream:
				if !ok {
					obj.Logf("stream closed")
					return
				}
				if err == nil {
					// only do this once, on the first event
					once.Do(loadedSignal) // signal
				}

			case <-obj.closeChan:
				return
			}

			select {
			case obj.streamChan <- err: // send
				if err != nil {
					obj.Logf("Stream error: %+v", err)
					return
				}

			case <-obj.closeChan:
				return
			}
		}
	}()
	return nil
}

// Stream returns a channel of graph change requests or errors. These are
// usually sent when a func output changes.
func (obj *Lang) Stream() chan error {
	return obj.streamChan
}

// Interpret runs the interpreter and returns a graph and corresponding error.
func (obj *Lang) Interpret() (*pgraph.Graph, error) {
	select {
	case <-obj.loadedChan: // funcs are now loaded!
		// pass
	default:
		// if this is hit, someone probably called this too early!
		// it should only be called in response to a stream event!
		return nil, fmt.Errorf("funcs aren't loaded yet")
	}

	obj.Logf("running interpret...")
	if obj.funcs != nil { // no need to rlock if we have a static graph
		obj.funcs.RLock()
	}
	// this call returns the graph
	graph, err := interpret(obj.ast)
	if obj.funcs != nil {
		obj.funcs.RUnlock()
	}
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpret")
	}

	return graph, nil // return a graph
}

// Close shuts down the lang struct and causes all the funcs to shutdown. It
// must be called when finished after any successful Init ran.
func (obj *Lang) Close() error {
	var err error
	if obj.funcs != nil {
		err = obj.funcs.Close()
	}
	close(obj.closeChan)
	obj.wg.Wait()
	return err
}

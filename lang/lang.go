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

// Package lang is the mcl language frontend that implements the reactive DSL
// that lets users model their desired state over time.
package lang

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/local"
	"github.com/purpleidea/mgmt/lang/ast"
	_ "github.com/purpleidea/mgmt/lang/core" // import so the funcs register
	"github.com/purpleidea/mgmt/lang/funcs/dage"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/interpolate"
	"github.com/purpleidea/mgmt/lang/interpret"
	"github.com/purpleidea/mgmt/lang/parser"
	"github.com/purpleidea/mgmt/lang/types"
	"github.com/purpleidea/mgmt/lang/unification"
	_ "github.com/purpleidea/mgmt/lang/unification/solvers" // import so the solvers register
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"
	"github.com/purpleidea/mgmt/util/errwrap"
)

const (
	// EngineStartupStatsTimeout is the amount of time in seconds to wait
	// between engine startup, and loaded event before printing some
	// debugging stats. This is useful for finding bugs in the function
	// engine. Set to zero to disable.
	EngineStartupStatsTimeout = 10
)

// Data is some data that is passed into the Lang struct. It is presented here
// as a single struct with room for multiple fields so that it can be changed or
// extended in the future without having to re-plumb through all the fields it
// contains
type Data struct {
	// UnificationStrategy is a hack to tune unification performance until
	// we have an overall cleaner unification algorithm in place.
	UnificationStrategy map[string]string

	// TODO: Add other fields here if necessary.
}

// Lang is the main language lexer/parser object.
type Lang struct {
	Fs    engine.Fs // connected fs where input dir or metadata exists
	FsURI string

	// Input is a string which specifies what the lang should run. It can
	// accept values in several different forms. If is passed a single dash
	// (-), then it will use `os.Stdin`. If it is passed a single .mcl file,
	// then it will attempt to run that. If it is passed a directory path,
	// then it will attempt to run from there. Instead, if it is passed the
	// path to a metadata file, then it will attempt to parse that and run
	// from that specification. If none of those match, it will attempt to
	// run the raw string as mcl code.
	Input string

	// Data is some additional data for the lang struct.
	Data *Data

	Hostname string
	Local    *local.API
	World    engine.World
	Prefix   string
	Debug    bool
	Logf     func(format string, v ...interface{})

	ast   interfaces.Stmt // store main prog AST here
	funcs *dage.Engine    // function event engine
	graph *pgraph.Graph   // function graph

	streamChan <-chan error // signals a new graph can be created or problem
	//streamBurst bool // should we try and be bursty with the stream events?

	wg *sync.WaitGroup
}

// Init initializes the lang struct, and starts up the initial input parsing.
// NOTE: The trick is that we need to get the list of funcs to watch AND start
// watching them, *before* we pull their values, that way we'll know if they
// changed from the values we wanted.
func (obj *Lang) Init(ctx context.Context) error {
	if obj.Data == nil {
		return fmt.Errorf("lang struct was not built properly")
	}
	if obj.Data.UnificationStrategy == nil {
		return fmt.Errorf("lang struct was not built properly")
	}
	if obj.Debug {
		obj.Logf("input: %s", obj.Input)
		tree, err := util.FsTree(obj.Fs, "/") // should look like gapi
		if err != nil {
			return err
		}
		obj.Logf("run tree:\n%s", tree)
	}
	var timing time.Time

	// we used to support stdin passthrough, but we we got rid of it for now
	// the fs input here is the local fs we're reading to get the files from
	// which is usually etcdFs.
	output, err := inputs.ParseInput(obj.Input, obj.Fs)
	if err != nil {
		return errwrap.Wrapf(err, "could not activate an input parser")
	}
	if len(output.Workers) > 0 {
		// either programming error, or someone hacked in something here
		// by the time *this* ParseInput runs, we should be standardized
		return fmt.Errorf("input contained file system workers")
	}
	reader := bytes.NewReader(output.Main)

	// no need to run recursion detection since this is the beginning
	// TODO: do the paths need to be cleaned for "../" before comparison?

	// run the lexer/parser and build an AST
	obj.Logf("lexing/parsing...")
	timing = time.Now()
	// this reads an io.Reader, which might be a stream of multiple files...
	xast, err := parser.LexParse(reader)
	if err != nil {
		return errwrap.Wrapf(err, "could not generate AST")
	}
	obj.Logf("lexing/parsing took: %s", time.Since(timing))
	if obj.Debug {
		obj.Logf("behold, the AST: %+v", xast)
	}

	importGraph, err := pgraph.NewGraph("importGraph")
	if err != nil {
		return err
	}
	importVertex := &pgraph.SelfVertex{
		Name:  "",          // first node is the empty string
		Graph: importGraph, // store a reference to ourself
	}
	importGraph.AddVertex(importVertex)

	obj.Logf("init...")
	// init and validate the structure of the AST
	data := &interfaces.Data{
		// TODO: add missing fields here if/when needed
		Fs:       output.FS,       // formerly: obj.Fs
		FsURI:    output.FS.URI(), // formerly: obj.FsURI
		Base:     output.Base,     // base dir (absolute path) the metadata file is in
		Files:    output.Files,
		Imports:  importVertex,
		Metadata: output.Metadata,
		Modules:  "/" + interfaces.ModuleDirectory, // do not set from env for a deploy!

		LexParser:       parser.LexParse,
		Downloader:      nil, // XXX: is this used here?
		StrInterpolater: interpolate.StrInterpolate,
		//Local: obj.Local, // TODO: do we need this?
		//World: obj.World, // TODO: do we need this?

		Prefix: obj.Prefix,
		Debug:  obj.Debug,
		Logf: func(format string, v ...interface{}) {
			// TODO: is this a sane prefix to use here?
			obj.Logf("ast: "+format, v...)
		},
	}
	// some of this might happen *after* interpolate in SetScope or Unify...
	if err := xast.Init(data); err != nil {
		return errwrap.Wrapf(err, "could not init and validate AST")
	}

	obj.Logf("interpolating...")
	timing = time.Now()
	// interpolate strings and other expansionable nodes in AST
	iast, err := xast.Interpolate()
	if err != nil {
		return errwrap.Wrapf(err, "could not interpolate AST")
	}
	obj.Logf("interpolating took: %s", time.Since(timing))
	obj.ast = iast

	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		// TODO: change to a func when we can change hostname dynamically!
		"hostname": &ast.ExprStr{V: obj.Hostname},
	}
	// TODO: pass `data` into ast.VarPrefixToVariablesScope ?
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		return errwrap.Wrapf(err, "couldn't merge in consts")
	}

	// top-level, built-in, initial global scope
	scope := &interfaces.Scope{
		Variables: variables,
		// all the built-in top-level, core functions enter here...
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}

	obj.Logf("scope building...")
	timing = time.Now()
	// propagate the scope down through the AST...
	if err := obj.ast.SetScope(scope); err != nil {
		return errwrap.Wrapf(err, "could not set scope")
	}
	obj.Logf("scope building took: %s", time.Since(timing))

	// apply type unification
	logf := func(format string, v ...interface{}) {
		// TODO: Remove the masked logger here when unification is clean!
		if obj.Debug { // unification only has debug messages...
			obj.Logf("unification: "+format, v...)
		}
	}
	obj.Logf("running type unification...")

	var solver unification.Solver
	if name, exists := obj.Data.UnificationStrategy["solver"]; exists && name != "" {
		if solver, err = unification.Lookup(name); err != nil {
			return errwrap.Wrapf(err, "could not get solver: %s", name)
		}
	} else if solver, err = unification.LookupDefault(); err != nil {
		return errwrap.Wrapf(err, "could not get default solver")
	}
	unifier := &unification.Unifier{
		AST:          obj.ast,
		Solver:       solver,
		Strategy:     obj.Data.UnificationStrategy,
		UnifiedState: types.NewUnifiedState(),
		Debug:        obj.Debug,
		Logf:         logf,
	}
	timing = time.Now()
	// NOTE: This is the "real" Unify that runs. (This is not for deploy.)
	unifyErr := unifier.Unify(ctx)
	obj.Logf("type unification took: %s", time.Since(timing))
	if unifyErr != nil {
		return errwrap.Wrapf(unifyErr, "could not unify types")
	}

	// XXX: Should we do a kind of SetType on resources here to tell the
	// ones with variant fields what their concrete field types are? They
	// should only be dynamic in implementation and before unification, and
	// static once we've unified the specific resource.

	obj.Logf("building function graph...")
	timing = time.Now()
	// we assume that for some given code, the list of funcs doesn't change
	// iow, we don't support variable, variables or absurd things like that
	obj.graph = &pgraph.Graph{Name: "functionGraph"}
	env := make(map[string]interfaces.Func)
	for k, v := range scope.Variables {
		g, builtinFunc, err := v.Graph(nil)
		if err != nil {
			return errwrap.Wrapf(err, "calling Graph on builtins")
		}
		obj.graph.AddGraph(g)
		env[k] = builtinFunc
	}
	g, err := obj.ast.Graph() // build the graph of functions
	if err != nil {
		return errwrap.Wrapf(err, "could not generate function graph")
	}
	obj.graph.AddGraph(g)

	if obj.Debug {
		obj.Logf("function graph: %+v", obj.graph)
		obj.graph.Logf(obj.Logf) // log graph output with this logger...
		//if err := obj.graph.ExecGraphviz("/tmp/graphviz.dot"); err != nil {
		//	return errwrap.Wrapf(err, "writing graph failed")
		//}
	}

	obj.funcs = &dage.Engine{
		Name:     "lang", // TODO: arbitrary name for now
		Hostname: obj.Hostname,
		Local:    obj.Local,
		World:    obj.World,
		//Prefix:   fmt.Sprintf("%s/", path.Join(obj.Prefix, "funcs")),
		Debug: obj.Debug,
		Logf: func(format string, v ...interface{}) {
			obj.Logf("funcs: "+format, v...)
		},
	}

	obj.Logf("function engine initializing...")
	if err := obj.funcs.Setup(); err != nil {
		return errwrap.Wrapf(err, "init error with func engine")
	}
	obj.Logf("function setup took: %s", time.Since(timing))

	obj.streamChan = obj.funcs.Stream() // after obj.funcs.Setup runs

	return nil
}

// Run kicks off the function engine. Use the context to shut it down.
func (obj *Lang) Run(ctx context.Context) (reterr error) {
	wg := &sync.WaitGroup{}
	defer wg.Wait()

	runCtx, cancel := context.WithCancel(context.Background()) // Don't inherit from parent
	defer cancel()

	//obj.Logf("function engine validating...")
	//if err := obj.funcs.Validate(); err != nil {
	//	return errwrap.Wrapf(err, "validate error with func engine")
	//}

	obj.Logf("function engine starting...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := obj.funcs.Run(runCtx); err == nil {
			reterr = errwrap.Append(reterr, err)
		}
		// Run() should only error if not a dag I think...
	}()

	<-obj.funcs.Started() // wait for startup (will not block forever)

	// Sanity checks for graph size.
	if count := obj.funcs.NumVertices(); count != 0 {
		return fmt.Errorf("expected empty graph on start, got %d vertices", count)
	}
	defer func() {
		if count := obj.funcs.NumVertices(); count != 0 {
			err := fmt.Errorf("expected empty graph on exit, got %d vertices", count)
			reterr = errwrap.Append(reterr, err)
		}
	}()
	defer wg.Wait()
	defer cancel() // now cancel Run only after Reverse and Free are done!

	txn := obj.funcs.Txn()
	defer txn.Free() // remember to call Free()
	txn.AddGraph(obj.graph)
	if err := txn.Commit(); err != nil {
		return errwrap.Wrapf(err, "error adding to function graph engine")
	}
	defer func() {
		if err := txn.Reverse(); err != nil { // should remove everything we added
			reterr = errwrap.Append(reterr, err)
		}
	}()

	// wait for some activity
	obj.Logf("stream...")

	// print some stats if the engine takes too long to startup
	if EngineStartupStatsTimeout > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-obj.funcs.Loaded(): // funcs are now loaded!
			case <-time.After(time.Duration(EngineStartupStatsTimeout) * time.Second):
				obj.Logf("stats...")
				obj.Logf("%s", obj.funcs.Stats())
			case <-ctx.Done():
			}
		}()
	}

	select {
	case <-ctx.Done():
	}

	return nil
}

// Stream returns a channel of graph change requests or errors. These are
// usually sent when a func output changes.
func (obj *Lang) Stream() <-chan error {
	return obj.streamChan
}

// Interpret runs the interpreter and returns a graph and corresponding error.
func (obj *Lang) Interpret() (*pgraph.Graph, error) {
	select {
	case <-obj.funcs.Loaded(): // funcs are now loaded!
		// pass
	default:
		// if this is hit, someone probably called this too early!
		// it should only be called in response to a stream event!
		return nil, fmt.Errorf("funcs aren't loaded yet")
	}

	obj.Logf("running interpret...")
	table := obj.funcs.Table() // map[pgraph.Vertex]types.Value

	// this call returns the graph
	graph, err := interpret.Interpret(obj.ast, table)
	if err != nil {
		return nil, errwrap.Wrapf(err, "could not interpret")
	}

	return graph, nil // return a graph
}

// Cleanup cleans up and frees memory and resources after everything is done.
func (obj *Lang) Cleanup() error {
	return obj.funcs.Cleanup()
}

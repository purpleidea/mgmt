// Mgmt
// Copyright (C) 2013-2023+ James Shubin and the project contributors
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

//go:build !root

package lang

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/purpleidea/mgmt/converger"
	"github.com/purpleidea/mgmt/engine"
	"github.com/purpleidea/mgmt/engine/graph"
	"github.com/purpleidea/mgmt/engine/graph/autoedge"
	"github.com/purpleidea/mgmt/engine/local"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/etcd"
	"github.com/purpleidea/mgmt/lang/ast"
	"github.com/purpleidea/mgmt/lang/funcs/dage"
	"github.com/purpleidea/mgmt/lang/funcs/vars"
	"github.com/purpleidea/mgmt/lang/inputs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/lang/interpolate"
	"github.com/purpleidea/mgmt/lang/interpret"
	"github.com/purpleidea/mgmt/lang/parser"
	"github.com/purpleidea/mgmt/lang/unification"
	"github.com/purpleidea/mgmt/pgraph"
	"github.com/purpleidea/mgmt/util"

	"github.com/kylelemons/godebug/pretty"
	"github.com/spf13/afero"
	"golang.org/x/tools/txtar"
)

const (
	runGraphviz = false // run graphviz in tests?
)

// TestAstFunc1 is a more advanced version which pulls code from physical dirs.
func TestAstFunc1(t *testing.T) {
	const magicError = "# err: "
	const magicErrorLexParse = "errLexParse: "
	const magicErrorInit = "errInit: "
	const magicErrorSetScope = "errSetScope: "
	const magicErrorUnify = "errUnify: "
	const magicErrorGraph = "errGraph: "
	const magicEmpty = "# empty!"
	dir, err := util.TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)

	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		// TODO: change to a func when we can change hostname dynamically!
		"hostname": &ast.ExprStr{V: ""}, // NOTE: empty b/c not used
	}
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		t.Errorf("couldn't merge in consts: %+v", err)
		return
	}

	scope := &interfaces.Scope{ // global scope
		Variables: variables,
		// all the built-in top-level, core functions enter here...
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}

	type test struct { // an individual test
		name string
		path string // relative txtar path inside tests dir
	}
	testCases := []test{}

	// build test array automatically from reading the dir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".txtar") {
			continue
		}

		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		// add automatic test case
		testCases = append(testCases, test{
			name: fmt.Sprintf("%s", f),
			path: f, // <something>.txtar
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, path := tc.name, tc.path
			tmpdir := t.TempDir() // gets cleaned up at end, new dir for each call
			src := tmpdir         // location of the test
			txtarFile := dir + path

			archive, err := txtar.ParseFile(txtarFile)
			if err != nil {
				t.Errorf("err parsing txtar(%s): %+v", txtarFile, err)
				return
			}
			comment := strings.TrimSpace(string(archive.Comment))
			t.Logf("comment: %s\n", comment)

			// copy files out into the test temp directory
			var testOutput []byte
			found := false
			for _, file := range archive.Files {
				if file.Name == "OUTPUT" {
					testOutput = file.Data
					found = true
					continue
				}

				name := filepath.Join(tmpdir, file.Name)
				dir := filepath.Dir(name)
				if err := os.MkdirAll(dir, 0770); err != nil {
					t.Errorf("err making dir(%s): %+v", dir, err)
					return
				}
				if err := ioutil.WriteFile(name, file.Data, 0660); err != nil {
					t.Errorf("err writing file(%s): %+v", name, err)
					return
				}
			}

			if !found { // skip missing tests
				return
			}

			expstr := string(testOutput) // expected graph

			// if the graph file has a magic error string, it's a failure
			errStr := ""
			failLexParse := false
			failInit := false
			failSetScope := false
			failUnify := false
			failGraph := false
			if strings.HasPrefix(expstr, magicError) {
				errStr = strings.TrimPrefix(expstr, magicError)
				expstr = errStr

				if strings.HasPrefix(expstr, magicErrorLexParse) {
					errStr = strings.TrimPrefix(expstr, magicErrorLexParse)
					expstr = errStr
					failLexParse = true
				}
				if strings.HasPrefix(expstr, magicErrorInit) {
					errStr = strings.TrimPrefix(expstr, magicErrorInit)
					expstr = errStr
					failInit = true
				}
				if strings.HasPrefix(expstr, magicErrorSetScope) {
					errStr = strings.TrimPrefix(expstr, magicErrorSetScope)
					expstr = errStr
					failSetScope = true
				}
				if strings.HasPrefix(expstr, magicErrorUnify) {
					errStr = strings.TrimPrefix(expstr, magicErrorUnify)
					expstr = errStr
					failUnify = true
				}
				if strings.HasPrefix(expstr, magicErrorGraph) {
					errStr = strings.TrimPrefix(expstr, magicErrorGraph)
					expstr = errStr
					failGraph = true
				}
			}

			fail := errStr != ""
			expstr = strings.Trim(expstr, "\n")

			t.Logf("\n\ntest #%d (%s) ----------------\npath: %s\n\n", index, name, src)

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			mmFs := afero.NewMemMapFs()
			afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
			fs := &util.Fs{Afero: afs}

			// use this variant, so that we don't copy the dir name
			// this is the equivalent to running `rsync -a src/ /`
			if err := util.CopyDiskContentsToFs(fs, src, "/", false); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: CopyDiskContentsToFs failed: %+v", index, err)
				return
			}

			// this shows us what we pulled in from the test dir:
			tree0, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree0)

			input := "/"
			logf("input: %s", input)

			output, err := inputs.ParseInput(input, fs) // raw code can be passed in
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseInput failed: %+v", index, err)
				return
			}
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: worker execution failed: %+v", index, err)
					return
				}
			}
			tree, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree)

			logf("main:\n%s", output.Main) // debug

			reader := bytes.NewReader(output.Main)
			xast, err := parser.LexParse(reader)
			if (!fail || !failLexParse) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if failLexParse && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failLexParse && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			t.Logf("test #%d: AST: %+v", index, xast)

			importGraph, err := pgraph.NewGraph("importGraph")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not create graph: %+v", index, err)
				return
			}
			importVertex := &pgraph.SelfVertex{
				Name:  "",          // first node is the empty string
				Graph: importGraph, // store a reference to ourself
			}
			importGraph.AddVertex(importVertex)

			data := &interfaces.Data{
				// TODO: add missing fields here if/when needed
				Fs:       fs,
				FsURI:    fs.URI(),     // TODO: is this right?
				Base:     output.Base,  // base dir (absolute path) the metadata file is in
				Files:    output.Files, // no really needed here afaict
				Imports:  importVertex,
				Metadata: output.Metadata,
				Modules:  "/" + interfaces.ModuleDirectory, // not really needed here afaict

				LexParser:       parser.LexParse,
				StrInterpolater: interpolate.StrInterpolate,

				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			err = xast.Init(data)
			if (!fail || !failInit) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}
			if failInit && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failInit && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			iast, err := xast.Interpolate()
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpolate failed with: %+v", index, err)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if (!fail || !failSetScope) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if failSetScope && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during set scope, don't run unification!
			}
			if failSetScope && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope passed, expected fail", index)
				return
			}

			// apply type unification
			xlogf := func(format string, v ...interface{}) {
				logf("unification: "+format, v...)
			}
			unifier := &unification.Unifier{
				AST:    iast,
				Solver: unification.SimpleInvariantSolverLogger(xlogf),
				Debug:  testing.Verbose(),
				Logf:   xlogf,
			}
			err = unifier.Unify()
			if (!fail || !failUnify) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			if failUnify && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during unification, don't run Graph!
			}
			if failUnify && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}

			// build the function graph
			fgraph, err := iast.Graph()
			if (!fail || !failGraph) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if failGraph && err != nil { // can't process graph if it's nil
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failGraph && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			t.Logf("test #%d: graph: %s", index, fgraph)
			for i, v := range fgraph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range fgraph.Adjacency() {
				for v2, e := range fgraph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}
			if runGraphviz {
				t.Logf("test #%d: Running graphviz...", index)
				if err := fgraph.ExecGraphviz("/tmp/graphviz.dot"); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: writing graph failed: %+v", index, err)
					return
				}
			}

			str := strings.Trim(fgraph.Sprint(), "\n") // text format of graph
			if expstr == magicEmpty {
				expstr = ""
			}
			// XXX: something isn't consistent, and I can't figure
			// out what, so workaround this by sorting these :(
			sortHack := func(x string) string {
				l := strings.Split(x, "\n")
				sort.Strings(l)
				return strings.Join(l, "\n")
			}
			str = sortHack(str)
			expstr = sortHack(expstr)
			if expstr != str {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1):\n%s\n\n", index, str)
				t.Logf("test #%d: expected (g2):\n%s\n\n", index, expstr)
				diff := pretty.Compare(str, expstr)
				if diff != "" { // bonus
					t.Logf("test #%d: diff:\n%s", index, diff)
				}
				return
			}

			for i, v := range fgraph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range fgraph.Adjacency() {
				for v2, e := range fgraph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}
		})
	}
	if testing.Short() {
		t.Skip("skipping all tests...")
	}
}

// TestAstFunc2 is a more advanced version which pulls code from physical dirs.
// It also briefly runs the function engine and captures output. Only use with
// stable, static output.
func TestAstFunc2(t *testing.T) {
	const magicError = "# err: "
	const magicErrorLexParse = "errLexParse: "
	const magicErrorInit = "errInit: "
	const magicInterpolate = "errInterpolate: "
	const magicErrorSetScope = "errSetScope: "
	const magicErrorUnify = "errUnify: "
	const magicErrorGraph = "errGraph: "
	const magicErrorStream = "errStream: "
	const magicErrorInterpret = "errInterpret: "
	const magicErrorAutoEdge = "errAutoEdge: "
	const magicEmpty = "# empty!"
	dir, err := util.TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)

	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		// TODO: change to a func when we can change hostname dynamically!
		"hostname": &ast.ExprStr{V: ""}, // NOTE: empty b/c not used
	}
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		t.Errorf("couldn't merge in consts: %+v", err)
		return
	}

	scope := &interfaces.Scope{ // global scope
		Variables: variables,
		// all the built-in top-level, core functions enter here...
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}

	type test struct { // an individual test
		name string
		path string // relative txtar path inside tests dir
	}
	testCases := []test{}

	// build test array automatically from reading the dir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".txtar") {
			continue
		}

		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		// add automatic test case
		testCases = append(testCases, test{
			name: fmt.Sprintf("%s", f),
			path: f, // <something>.txtar
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, path := tc.name, tc.path
			tmpdir := t.TempDir() // gets cleaned up at end, new dir for each call
			src := tmpdir         // location of the test
			txtarFile := dir + path

			archive, err := txtar.ParseFile(txtarFile)
			if err != nil {
				t.Errorf("err parsing txtar(%s): %+v", txtarFile, err)
				return
			}
			comment := strings.TrimSpace(string(archive.Comment))
			t.Logf("comment: %s\n", comment)

			// copy files out into the test temp directory
			var testOutput []byte
			found := false
			for _, file := range archive.Files {
				if file.Name == "OUTPUT" {
					testOutput = file.Data
					found = true
					continue
				}

				name := filepath.Join(tmpdir, file.Name)
				dir := filepath.Dir(name)
				if err := os.MkdirAll(dir, 0770); err != nil {
					t.Errorf("err making dir(%s): %+v", dir, err)
					return
				}
				if err := ioutil.WriteFile(name, file.Data, 0660); err != nil {
					t.Errorf("err writing file(%s): %+v", name, err)
					return
				}
			}

			if !found { // skip missing tests
				return
			}

			expstr := string(testOutput) // expected graph

			// if the graph file has a magic error string, it's a failure
			errStr := ""
			failLexParse := false
			failInit := false
			failInterpolate := false
			failSetScope := false
			failUnify := false
			failGraph := false
			failStream := false
			failInterpret := false
			failAutoEdge := false
			if strings.HasPrefix(expstr, magicError) {
				errStr = strings.TrimPrefix(expstr, magicError)
				expstr = errStr

				if strings.HasPrefix(expstr, magicErrorLexParse) {
					errStr = strings.TrimPrefix(expstr, magicErrorLexParse)
					expstr = errStr
					failLexParse = true
				}
				if strings.HasPrefix(expstr, magicErrorInit) {
					errStr = strings.TrimPrefix(expstr, magicErrorInit)
					expstr = errStr
					failInit = true
				}
				if strings.HasPrefix(expstr, magicInterpolate) {
					errStr = strings.TrimPrefix(expstr, magicInterpolate)
					expstr = errStr
					failInterpolate = true
				}
				if strings.HasPrefix(expstr, magicErrorSetScope) {
					errStr = strings.TrimPrefix(expstr, magicErrorSetScope)
					expstr = errStr
					failSetScope = true
				}
				if strings.HasPrefix(expstr, magicErrorUnify) {
					errStr = strings.TrimPrefix(expstr, magicErrorUnify)
					expstr = errStr
					failUnify = true
				}
				if strings.HasPrefix(expstr, magicErrorGraph) {
					errStr = strings.TrimPrefix(expstr, magicErrorGraph)
					expstr = errStr
					failGraph = true
				}
				if strings.HasPrefix(expstr, magicErrorStream) {
					errStr = strings.TrimPrefix(expstr, magicErrorStream)
					expstr = errStr
					failStream = true
				}
				if strings.HasPrefix(expstr, magicErrorInterpret) {
					errStr = strings.TrimPrefix(expstr, magicErrorInterpret)
					expstr = errStr
					failInterpret = true
				}
				if strings.HasPrefix(expstr, magicErrorAutoEdge) {
					errStr = strings.TrimPrefix(expstr, magicErrorAutoEdge)
					expstr = errStr
					failAutoEdge = true
				}
			}

			fail := errStr != ""
			expstr = strings.Trim(expstr, "\n")

			t.Logf("\n\ntest #%d (%s) ----------------\npath: %s\n\n", index, name, src)

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			mmFs := afero.NewMemMapFs()
			afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
			fs := &util.Fs{Afero: afs}

			// implementation of the Local API (we only expect just this single one)
			localAPI := &local.API{
				Prefix: fmt.Sprintf("%s/", filepath.Join(tmpdir, "local")),
				Debug:  testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("local: api: "+format, v...)
				},
			}

			// implementation of the World API (alternatives can be substituted in)
			world := &etcd.World{
				//Hostname:       hostname,
				//Client:         etcdClient,
				//MetadataPrefix: /fs, // MetadataPrefix
				//StoragePrefix:  "/storage", // StoragePrefix
				// TODO: is this correct? (seems to work for testing)
				StandaloneFs: fs,                // used for static deploys
				Debug:        testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("world: etcd: "+format, v...)
				},
			}

			// use this variant, so that we don't copy the dir name
			// this is the equivalent to running `rsync -a src/ /`
			if err := util.CopyDiskContentsToFs(fs, src, "/", false); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: CopyDiskContentsToFs failed: %+v", index, err)
				return
			}

			// this shows us what we pulled in from the test dir:
			tree0, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree0)

			input := "/"
			logf("input: %s", input)

			output, err := inputs.ParseInput(input, fs) // raw code can be passed in
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseInput failed: %+v", index, err)
				return
			}
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: worker execution failed: %+v", index, err)
					return
				}
			}
			tree, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree)

			logf("main:\n%s", output.Main) // debug

			reader := bytes.NewReader(output.Main)
			xast, err := parser.LexParse(reader)
			if (!fail || !failLexParse) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if failLexParse && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failLexParse && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			t.Logf("test #%d: AST: %+v", index, xast)

			importGraph, err := pgraph.NewGraph("importGraph")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not create graph: %+v", index, err)
				return
			}
			importVertex := &pgraph.SelfVertex{
				Name:  "",          // first node is the empty string
				Graph: importGraph, // store a reference to ourself
			}
			importGraph.AddVertex(importVertex)

			data := &interfaces.Data{
				// TODO: add missing fields here if/when needed
				Fs:       fs,
				FsURI:    "memmapfs:///", // we're in standalone mode
				Base:     output.Base,    // base dir (absolute path) the metadata file is in
				Files:    output.Files,   // no really needed here afaict
				Imports:  importVertex,
				Metadata: output.Metadata,
				Modules:  "/" + interfaces.ModuleDirectory, // not really needed here afaict

				LexParser:       parser.LexParse,
				StrInterpolater: interpolate.StrInterpolate,

				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			err = xast.Init(data)
			if (!fail || !failInit) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}
			if failInit && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failInit && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Init passed, expected fail", index)
				return
			}

			iast, err := xast.Interpolate()
			if (!fail || !failInterpolate) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Interpolate failed with: %+v", index, err)
				return
			}
			if failInterpolate && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failInterpolate && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Interpolate passed, expected fail", index)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if (!fail || !failSetScope) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if failSetScope && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during set scope, don't run unification!
			}
			if failSetScope && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope passed, expected fail", index)
				return
			}

			if runGraphviz {
				t.Logf("test #%d: Running graphviz after setScope...", index)

				// build a graph of the AST, to make sure everything is connected properly
				graph, err := pgraph.NewGraph("setScope")
				if err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not create setScope graph: %+v", index, err)
					return
				}
				ast, ok := iast.(interfaces.ScopeGrapher)
				if !ok {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: can't graph scope", index)
					return
				}
				ast.ScopeGraph(graph)

				if err := graph.ExecGraphviz("/tmp/set-scope.dot"); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: writing graph failed: %+v", index, err)
					return
				}
			}

			// apply type unification
			xlogf := func(format string, v ...interface{}) {
				logf("unification: "+format, v...)
			}
			unifier := &unification.Unifier{
				AST:    iast,
				Solver: unification.SimpleInvariantSolverLogger(xlogf),
				Debug:  testing.Verbose(),
				Logf:   xlogf,
			}
			err = unifier.Unify()
			if (!fail || !failUnify) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			if failUnify && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during unification, don't run Graph!
			}
			if failUnify && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}
			// XXX: Should we do a kind of SetType on resources here
			// to tell the ones with variant fields what their
			// concrete field types are? They should only be dynamic
			// in implementation and before unification, and static
			// once we've unified the specific resource.

			// build the function graph
			fgraph, err := iast.Graph()
			if (!fail || !failGraph) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if failGraph && err != nil { // can't process graph if it's nil
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failGraph && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			if fgraph.NumVertices() == 0 { // no funcs to load!
				//t.Errorf("test #%d: FAIL", index)
				t.Logf("test #%d: function graph is empty", index)
				//return // let's test the engine on empty
			}

			t.Logf("test #%d: graph: %s", index, fgraph)
			for i, v := range fgraph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range fgraph.Adjacency() {
				for v2, e := range fgraph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}

			if runGraphviz {
				t.Logf("test #%d: Running graphviz...", index)
				if err := fgraph.ExecGraphviz("/tmp/graphviz.dot"); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: writing graph failed: %+v", index, err)
					return
				}
			}

			// run the function engine once to get some real output
			funcs := &dage.Engine{
				Name:     "test",
				Hostname: "",                // NOTE: empty b/c not used
				Local:    localAPI,          // used partially in some tests
				World:    world,             // used partially in some tests
				//Prefix:   fmt.Sprintf("%s/", filepath.Join(tmpdir, "funcs")),
				Debug:    testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("funcs: "+format, v...)
				},
			}

			logf("function engine initializing...")
			if err := funcs.Setup(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: init error with func engine: %+v", index, err)
				return
			}
			defer funcs.Cleanup()

			// XXX: can we type check things somehow?
			//logf("function engine validating...")
			//if err := funcs.Validate(); err != nil {
			//	t.Errorf("test #%d: FAIL", index)
			//	t.Errorf("test #%d: validate error with func engine: %+v", index, err)
			//	return
			//}

			logf("function engine starting...")
			wg := &sync.WaitGroup{}
			defer wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := funcs.Run(ctx); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: run error with func engine: %+v", index, err)
					return
				}
			}()

			//wg.Add(1)
			//go func() { // XXX: debugging
			//	defer wg.Done()
			//	for {
			//		select {
			//		case <-time.After(100 * time.Millisecond): // blocked functions
			//			t.Logf("test #%d: graphviz...", index)
			//			funcs.Graphviz("") // log to /tmp/...
			//
			//		case <-ctx.Done():
			//			return
			//		}
			//	}
			//}()

			<-funcs.Started() // wait for startup (will not block forever)

			// Sanity checks for graph size.
			if count := funcs.NumVertices(); count != 0 {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected empty graph on start, got %d vertices", index, count)
			}
			defer func() {
				if count := funcs.NumVertices(); count != 0 {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected empty graph on exit, got %d vertices", index, count)
				}
			}()
			defer wg.Wait()
			defer cancel()

			txn := funcs.Txn()
			defer txn.Free() // remember to call Free()
			txn.AddGraph(fgraph)
			if err := txn.Commit(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: run error with initial commit: %+v", index, err)
				return
			}
			defer txn.Reverse() // should remove everything we added

			isEmpty := make(chan struct{})
			if fgraph.NumVertices() == 0 { // no funcs to load!
				close(isEmpty)
			}

			// wait for some activity
			logf("stream...")
			stream := funcs.Stream()
			//select {
			//case err, ok := <-stream:
			//	if !ok {
			//		t.Errorf("test #%d: FAIL", index)
			//		t.Errorf("test #%d: stream closed", index)
			//		return
			//	}
			//	if err != nil {
			//		t.Errorf("test #%d: FAIL", index)
			//		t.Errorf("test #%d: stream errored: %+v", index, err)
			//		return
			//	}
			//
			//case <-time.After(60 * time.Second): // blocked functions
			//	t.Errorf("test #%d: FAIL", index)
			//	t.Errorf("test #%d: stream timeout", index)
			//	return
			//}

			// sometimes the <-stream seems to constantly (or for a
			// long time?) win the races against the <-time.After(),
			// so add some limit to how many times we need to stream
			max := 1
		Loop:
			for {
				select {
				case err, ok := <-stream:
					if !ok {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: stream closed", index)
						return
					}
					if err != nil {
						if (!fail || !failStream) && err != nil {
							t.Errorf("test #%d: FAIL", index)
							t.Errorf("test #%d: stream errored: %+v", index, err)
							return
						}
						if failStream && err != nil {
							t.Logf("test #%d: stream errored: %+v", index, err)
							// Stream errors often have pointers in them, so don't compare for now.
							//s := err.Error() // convert to string
							//if s != expstr {
							//	t.Errorf("test #%d: FAIL", index)
							//	t.Errorf("test #%d: expected different error", index)
							//	t.Logf("test #%d: err: %s", index, s)
							//	t.Logf("test #%d: exp: %s", index, expstr)
							//}
							return
						}
						if failStream && err == nil {
							t.Errorf("test #%d: FAIL", index)
							t.Errorf("test #%d: stream passed, expected fail", index)
							return
						}
						return
					}
					t.Logf("test #%d: got stream event!", index)
					max--
					if max == 0 {
						break Loop
					}

				case <-isEmpty:
					break Loop

				case <-time.After(10 * time.Second): // blocked functions
					t.Errorf("test #%d: unblocking because no event was sent by the function engine for a while", index)
					break Loop

				case <-time.After(60 * time.Second): // blocked functions
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: stream timeout", index)
					return
				}
			}

			t.Logf("test #%d: %s", index, funcs.Stats())

			// run interpret!
			table := funcs.Table() // map[interfaces.Func]types.Value

			ograph, err := interpret.Interpret(iast, table)
			if (!fail || !failInterpret) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret failed with: %+v", index, err)
				return
			}
			if failInterpret && err != nil { // can't process graph if it's nil
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failInterpret && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret passed, expected fail", index)
				return
			}

			// add automatic edges...
			err = autoedge.AutoEdge(ograph, testing.Verbose(), logf)
			if (!fail || !failAutoEdge) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: automatic edges failed with: %+v", index, err)
				return
			}
			if failAutoEdge && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failAutoEdge && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: automatic edges passed, expected fail", index)
				return
			}

			// TODO: perform autogrouping?

			t.Logf("test #%d: graph: %+v", index, ograph)
			str := strings.Trim(ograph.Sprint(), "\n") // text format of output graph
			if expstr == magicEmpty {
				expstr = ""
			}
			// XXX: something isn't consistent, and I can't figure
			// out what, so workaround this by sorting these :(
			sortHack := func(x string) string {
				l := strings.Split(x, "\n")
				sort.Strings(l)
				return strings.Join(l, "\n")
			}
			str = sortHack(str)
			expstr = sortHack(expstr)
			if expstr != str {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1):\n%s\n\n", index, str)
				t.Logf("test #%d: expected (g2):\n%s\n\n", index, expstr)
				diff := pretty.Compare(str, expstr)
				if diff != "" { // bonus
					t.Logf("test #%d: diff:\n%s", index, diff)
				}
				return
			}

			for i, v := range ograph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range ograph.Adjacency() {
				for v2, e := range ograph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}

			if !t.Failed() {
				t.Logf("test #%d: Passed!", index)
			}
		})
	}
	if testing.Short() {
		t.Skip("skipping all tests...")
	}
}

// TestAstFunc3 is an even more advanced version which also examines parameter
// values. It briefly runs the function engine and captures output. Only use
// with stable, static output. It also briefly runs the resource engine too!
func TestAstFunc3(t *testing.T) {
	const magicError = "# err: "
	const magicErrorLexParse = "errLexParse: "
	const magicErrorInit = "errInit: "
	const magicInterpolate = "errInterpolate: "
	const magicErrorSetScope = "errSetScope: "
	const magicErrorUnify = "errUnify: "
	const magicErrorGraph = "errGraph: "
	const magicErrorInterpret = "errInterpret: "
	const magicErrorAutoEdge = "errAutoEdge: "
	const magicEmpty = "# empty!"
	dir, err := util.TestDirFull()
	if err != nil {
		t.Errorf("could not get tests directory: %+v", err)
		return
	}
	t.Logf("tests directory is: %s", dir)

	variables := map[string]interfaces.Expr{
		"purpleidea": &ast.ExprStr{V: "hello world!"}, // james says hi
		// TODO: change to a func when we can change hostname dynamically!
		"hostname": &ast.ExprStr{V: ""}, // NOTE: empty b/c not used
	}
	consts := ast.VarPrefixToVariablesScope(vars.ConstNamespace) // strips prefix!
	addback := vars.ConstNamespace + interfaces.ModuleSep        // add it back...
	variables, err = ast.MergeExprMaps(variables, consts, addback)
	if err != nil {
		t.Errorf("couldn't merge in consts: %+v", err)
		return
	}

	scope := &interfaces.Scope{ // global scope
		Variables: variables,
		// all the built-in top-level, core functions enter here...
		Functions: ast.FuncPrefixToFunctionsScope(""), // runs funcs.LookupPrefix
	}

	type test struct { // an individual test
		name string
		path string // relative txtar path inside tests dir
	}
	testCases := []test{}

	// build test array automatically from reading the dir
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Errorf("could not read through tests directory: %+v", err)
		return
	}
	sorted := []string{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".txtar") {
			continue
		}

		sorted = append(sorted, f.Name())
	}
	sort.Strings(sorted)
	for _, f := range sorted {
		// add automatic test case
		testCases = append(testCases, test{
			name: fmt.Sprintf("%s", f),
			path: f, // <something>.txtar
		})
	}

	if testing.Short() {
		t.Logf("available tests:")
	}
	names := []string{}
	for index, tc := range testCases { // run all the tests
		if tc.name == "" {
			t.Errorf("test #%d: not named", index)
			continue
		}
		if util.StrInList(tc.name, names) {
			t.Errorf("test #%d: duplicate sub test name of: %s", index, tc.name)
			continue
		}
		names = append(names, tc.name)

		//if index != 3 { // hack to run a subset (useful for debugging)
		//if tc.name != "simple operators" {
		//	continue
		//}

		testName := fmt.Sprintf("test #%d (%s)", index, tc.name)
		if testing.Short() { // make listing tests easier
			t.Logf("%s", testName)
			continue
		}
		t.Run(testName, func(t *testing.T) {
			name, path := tc.name, tc.path
			tmpdir := t.TempDir() // gets cleaned up at end, new dir for each call
			src := tmpdir         // location of the test
			txtarFile := dir + path

			archive, err := txtar.ParseFile(txtarFile)
			if err != nil {
				t.Errorf("err parsing txtar(%s): %+v", txtarFile, err)
				return
			}
			comment := strings.TrimSpace(string(archive.Comment))
			t.Logf("comment: %s\n", comment)

			// copy files out into the test temp directory
			var testOutput []byte
			found := false
			for _, file := range archive.Files {
				if file.Name == "OUTPUT" {
					testOutput = file.Data
					found = true
					continue
				}

				name := filepath.Join(tmpdir, file.Name)
				dir := filepath.Dir(name)
				if err := os.MkdirAll(dir, 0770); err != nil {
					t.Errorf("err making dir(%s): %+v", dir, err)
					return
				}
				if err := ioutil.WriteFile(name, file.Data, 0660); err != nil {
					t.Errorf("err writing file(%s): %+v", name, err)
					return
				}
			}

			if !found { // skip missing tests
				return
			}

			expstr := string(testOutput) // expected graph

			// if the graph file has a magic error string, it's a failure
			errStr := ""
			failLexParse := false
			failInit := false
			failInterpolate := false
			failSetScope := false
			failUnify := false
			failGraph := false
			failInterpret := false
			failAutoEdge := false
			if strings.HasPrefix(expstr, magicError) {
				errStr = strings.TrimPrefix(expstr, magicError)
				expstr = errStr

				if strings.HasPrefix(expstr, magicErrorLexParse) {
					errStr = strings.TrimPrefix(expstr, magicErrorLexParse)
					expstr = errStr
					failLexParse = true
				}
				if strings.HasPrefix(expstr, magicErrorInit) {
					errStr = strings.TrimPrefix(expstr, magicErrorInit)
					expstr = errStr
					failInit = true
				}
				if strings.HasPrefix(expstr, magicInterpolate) {
					errStr = strings.TrimPrefix(expstr, magicInterpolate)
					expstr = errStr
					failInterpolate = true
				}
				if strings.HasPrefix(expstr, magicErrorSetScope) {
					errStr = strings.TrimPrefix(expstr, magicErrorSetScope)
					expstr = errStr
					failSetScope = true
				}
				if strings.HasPrefix(expstr, magicErrorUnify) {
					errStr = strings.TrimPrefix(expstr, magicErrorUnify)
					expstr = errStr
					failUnify = true
				}
				if strings.HasPrefix(expstr, magicErrorGraph) {
					errStr = strings.TrimPrefix(expstr, magicErrorGraph)
					expstr = errStr
					failGraph = true
				}
				if strings.HasPrefix(expstr, magicErrorInterpret) {
					errStr = strings.TrimPrefix(expstr, magicErrorInterpret)
					expstr = errStr
					failInterpret = true
				}
				if strings.HasPrefix(expstr, magicErrorAutoEdge) {
					errStr = strings.TrimPrefix(expstr, magicErrorAutoEdge)
					expstr = errStr
					failAutoEdge = true
				}
			}

			fail := errStr != ""
			expstr = strings.Trim(expstr, "\n")

			t.Logf("\n\ntest #%d (%s) ----------------\npath: %s\n\n", index, name, src)

			logf := func(format string, v ...interface{}) {
				t.Logf(fmt.Sprintf("test #%d", index)+": "+format, v...)
			}
			mmFs := afero.NewMemMapFs()
			afs := &afero.Afero{Fs: mmFs} // wrap so that we're implementing ioutil
			fs := &util.Fs{Afero: afs}

			// implementation of the Local API (we only expect just this single one)
			localAPI := &local.API{
				Prefix: fmt.Sprintf("%s/", filepath.Join(tmpdir, "local")),
				Debug:  testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("local: api: "+format, v...)
				},
			}

			// implementation of the World API (alternatives can be substituted in)
			world := &etcd.World{
				//Hostname:       hostname,
				//Client:         etcdClient,
				//MetadataPrefix: /fs, // MetadataPrefix
				//StoragePrefix:  "/storage", // StoragePrefix
				// TODO: is this correct? (seems to work for testing)
				StandaloneFs: fs,                // used for static deploys
				Debug:        testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("world: etcd: "+format, v...)
				},
			}

			// use this variant, so that we don't copy the dir name
			// this is the equivalent to running `rsync -a src/ /`
			if err := util.CopyDiskContentsToFs(fs, src, "/", false); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: CopyDiskContentsToFs failed: %+v", index, err)
				return
			}

			// this shows us what we pulled in from the test dir:
			tree0, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree0)

			input := "/"
			logf("input: %s", input)

			output, err := inputs.ParseInput(input, fs) // raw code can be passed in
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: ParseInput failed: %+v", index, err)
				return
			}
			for _, fn := range output.Workers {
				if err := fn(fs); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: worker execution failed: %+v", index, err)
					return
				}
			}
			tree, err := util.FsTree(fs, "/")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: FsTree failed: %+v", index, err)
				return
			}
			logf("tree:\n%s", tree)

			logf("main:\n%s", output.Main) // debug

			reader := bytes.NewReader(output.Main)
			xast, err := parser.LexParse(reader)
			if (!fail || !failLexParse) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse failed with: %+v", index, err)
				return
			}
			if failLexParse && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failLexParse && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: lex/parse passed, expected fail", index)
				return
			}

			t.Logf("test #%d: AST: %+v", index, xast)

			importGraph, err := pgraph.NewGraph("importGraph")
			if err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not create graph: %+v", index, err)
				return
			}
			importVertex := &pgraph.SelfVertex{
				Name:  "",          // first node is the empty string
				Graph: importGraph, // store a reference to ourself
			}
			importGraph.AddVertex(importVertex)

			data := &interfaces.Data{
				// TODO: add missing fields here if/when needed
				Fs:       fs,
				FsURI:    "memmapfs:///", // we're in standalone mode
				Base:     output.Base,    // base dir (absolute path) the metadata file is in
				Files:    output.Files,   // no really needed here afaict
				Imports:  importVertex,
				Metadata: output.Metadata,
				Modules:  "/" + interfaces.ModuleDirectory, // not really needed here afaict

				LexParser:       parser.LexParse,
				StrInterpolater: interpolate.StrInterpolate,

				Debug: testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("ast: "+format, v...)
				},
			}
			// some of this might happen *after* interpolate in SetScope or Unify...
			err = xast.Init(data)
			if (!fail || !failInit) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not init and validate AST: %+v", index, err)
				return
			}
			if failInit && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failInit && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Init passed, expected fail", index)
				return
			}

			iast, err := xast.Interpolate()
			if (!fail || !failInterpolate) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Interpolate failed with: %+v", index, err)
				return
			}
			if failInterpolate && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during lex parse, don't run init/interpolate!
			}
			if failInterpolate && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: Interpolate passed, expected fail", index)
				return
			}

			// propagate the scope down through the AST...
			err = iast.SetScope(scope)
			if (!fail || !failSetScope) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not set scope: %+v", index, err)
				return
			}
			if failSetScope && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during set scope, don't run unification!
			}
			if failSetScope && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: set scope passed, expected fail", index)
				return
			}

			if runGraphviz {
				t.Logf("test #%d: Running graphviz after setScope...", index)

				// build a graph of the AST, to make sure everything is connected properly
				graph, err := pgraph.NewGraph("setScope")
				if err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: could not create setScope graph: %+v", index, err)
					return
				}
				ast, ok := iast.(interfaces.ScopeGrapher)
				if !ok {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: can't graph scope", index)
					return
				}
				ast.ScopeGraph(graph)

				if err := graph.ExecGraphviz("/tmp/set-scope.dot"); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: writing graph failed: %+v", index, err)
					return
				}
			}

			// apply type unification
			xlogf := func(format string, v ...interface{}) {
				logf("unification: "+format, v...)
			}
			unifier := &unification.Unifier{
				AST:    iast,
				Solver: unification.SimpleInvariantSolverLogger(xlogf),
				Debug:  testing.Verbose(),
				Logf:   xlogf,
			}
			err = unifier.Unify()
			if (!fail || !failUnify) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: could not unify types: %+v", index, err)
				return
			}
			if failUnify && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return // fail happened during unification, don't run Graph!
			}
			if failUnify && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: unification passed, expected fail", index)
				return
			}
			// XXX: Should we do a kind of SetType on resources here
			// to tell the ones with variant fields what their
			// concrete field types are? They should only be dynamic
			// in implementation and before unification, and static
			// once we've unified the specific resource.

			// build the function graph
			fgraph, err := iast.Graph()
			if (!fail || !failGraph) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions failed with: %+v", index, err)
				return
			}
			if failGraph && err != nil { // can't process graph if it's nil
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failGraph && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: functions passed, expected fail", index)
				return
			}

			if fgraph.NumVertices() == 0 { // no funcs to load!
				//t.Errorf("test #%d: FAIL", index)
				t.Logf("test #%d: function graph is empty", index)
				//return // let's test the engine on empty
			}

			t.Logf("test #%d: graph: %s", index, fgraph)
			for i, v := range fgraph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range fgraph.Adjacency() {
				for v2, e := range fgraph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}

			if runGraphviz {
				t.Logf("test #%d: Running graphviz...", index)
				if err := fgraph.ExecGraphviz("/tmp/graphviz.dot"); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: writing graph failed: %+v", index, err)
					return
				}
			}

			// run the function engine once to get some real output
			funcs := &dage.Engine{
				Name:     "test",
				Hostname: "",                // NOTE: empty b/c not used
				Local:    localAPI,          // used partially in some tests
				World:    world,             // used partially in some tests
				//Prefix:   fmt.Sprintf("%s/", filepath.Join(tmpdir, "funcs")),
				Debug:    testing.Verbose(), // set via the -test.v flag to `go test`
				Logf: func(format string, v ...interface{}) {
					logf("funcs: "+format, v...)
				},
			}

			logf("function engine initializing...")
			if err := funcs.Setup(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: init error with func engine: %+v", index, err)
				return
			}
			defer funcs.Cleanup()

			// XXX: can we type check things somehow?
			//logf("function engine validating...")
			//if err := funcs.Validate(); err != nil {
			//	t.Errorf("test #%d: FAIL", index)
			//	t.Errorf("test #%d: validate error with func engine: %+v", index, err)
			//	return
			//}

			logf("function engine starting...")
			wg := &sync.WaitGroup{}
			defer wg.Wait()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := funcs.Run(ctx); err != nil {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: run error with func engine: %+v", index, err)
					return
				}
			}()

			//wg.Add(1)
			//go func() { // XXX: debugging
			//	defer wg.Done()
			//	for {
			//		select {
			//		case <-time.After(100 * time.Millisecond): // blocked functions
			//			t.Logf("test #%d: graphviz...", index)
			//			funcs.Graphviz("") // log to /tmp/...
			//
			//		case <-ctx.Done():
			//			return
			//		}
			//	}
			//}()

			<-funcs.Started() // wait for startup (will not block forever)

			// Sanity checks for graph size.
			if count := funcs.NumVertices(); count != 0 {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: expected empty graph on start, got %d vertices", index, count)
			}
			defer func() {
				if count := funcs.NumVertices(); count != 0 {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected empty graph on exit, got %d vertices", index, count)
				}
			}()
			defer wg.Wait()
			defer cancel()

			txn := funcs.Txn()
			defer txn.Free() // remember to call Free()
			txn.AddGraph(fgraph)
			if err := txn.Commit(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: run error with initial commit: %+v", index, err)
				return
			}
			defer txn.Reverse() // should remove everything we added

			isEmpty := make(chan struct{})
			if fgraph.NumVertices() == 0 { // no funcs to load!
				close(isEmpty)
			}

			// wait for some activity
			logf("stream...")
			stream := funcs.Stream()
			//select {
			//case err, ok := <-stream:
			//	if !ok {
			//		t.Errorf("test #%d: FAIL", index)
			//		t.Errorf("test #%d: stream closed", index)
			//		return
			//	}
			//	if err != nil {
			//		t.Errorf("test #%d: FAIL", index)
			//		t.Errorf("test #%d: stream errored: %+v", index, err)
			//		return
			//	}
			//
			//case <-time.After(60 * time.Second): // blocked functions
			//	t.Errorf("test #%d: FAIL", index)
			//	t.Errorf("test #%d: stream timeout", index)
			//	return
			//}

			// sometimes the <-stream seems to constantly (or for a
			// long time?) win the races against the <-time.After(),
			// so add some limit to how many times we need to stream
			max := 1
		Loop:
			for {
				select {
				case err, ok := <-stream:
					if !ok {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: stream closed", index)
						return
					}
					if err != nil {
						t.Errorf("test #%d: FAIL", index)
						t.Errorf("test #%d: stream errored: %+v", index, err)
						return
					}
					t.Logf("test #%d: got stream event!", index)
					max--
					if max == 0 {
						break Loop
					}

				case <-isEmpty:
					break Loop

				case <-time.After(10 * time.Second): // blocked functions
					t.Errorf("test #%d: unblocking because no event was sent by the function engine for a while", index)
					break Loop

				case <-time.After(60 * time.Second): // blocked functions
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: stream timeout", index)
					return
				}
			}

			t.Logf("test #%d: %s", index, funcs.Stats())

			// run interpret!
			table := funcs.Table() // map[interfaces.Func]types.Value

			ograph, err := interpret.Interpret(iast, table)
			if (!fail || !failInterpret) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret failed with: %+v", index, err)
				return
			}
			if failInterpret && err != nil { // can't process graph if it's nil
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failInterpret && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: interpret passed, expected fail", index)
				return
			}

			// add automatic edges...
			// TODO: use ge.AutoEdge() instead?
			err = autoedge.AutoEdge(ograph, testing.Verbose(), logf)
			if (!fail || !failAutoEdge) && err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: automatic edges failed with: %+v", index, err)
				return
			}
			if failAutoEdge && err != nil {
				s := err.Error() // convert to string
				if s != expstr {
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: expected different error", index)
					t.Logf("test #%d: err: %s", index, s)
					t.Logf("test #%d: exp: %s", index, expstr)
				}
				return
			}
			if failAutoEdge && err == nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: automatic edges passed, expected fail", index)
				return
			}

			// TODO: perform autogrouping?

			// TODO: perform reversals?

			t.Logf("test #%d: graph: %+v", index, ograph)

			// setup converger
			convergedTimeout := int64(5)
			converger := converger.New(
				convergedTimeout,
			)
			converged := make(chan struct{})
			converger.AddStateFn("converged-exit", func(isConverged bool) error {
				if isConverged {
					logf("converged for %d seconds, exiting!", convergedTimeout)
					close(converged) // trigger an exit!
				}
				return nil
			})

			// TODO: waitgroup ?
			go converger.Run(true) // main loop for converger, true to start paused
			converger.Ready()      // block until ready
			defer func() {
				// TODO: shutdown converger, but make sure that using it in a
				// still running embdEtcd struct doesn't block waiting on it...
				converger.Shutdown()
			}()

			// run engine a bit so that send/recv happens
			ge := &graph.Engine{
				Program: "testing", // TODO: name it mgmt?
				//Version:   obj.Version,
				Hostname:  "localhost",
				Converger: converger,
				Local:     localAPI,
				World:     world,
				Prefix:    fmt.Sprintf("%s/", filepath.Join(tmpdir, "engine")),
				Debug:     testing.Verbose(),
				Logf: func(format string, v ...interface{}) {
					logf("engine: "+format, v...)
				},
			}
			if err := ge.Init(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: engine Init failed with: %+v", index, err)
				return
			}
			defer func() {
				if err := ge.Shutdown(); err != nil {
					// TODO: cause the final exit code to be non-zero
					t.Errorf("test #%d: FAIL", index)
					t.Errorf("test #%d: engine Shutdown failed with: %+v", index, err)
					return
				}
			}()

			if err := ge.Load(ograph); err != nil { // copy in new graph
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: error copying in new graph: %+v", index, err)
				return
			}

			if err := ge.Validate(); err != nil { // validate the new graph
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: error validating the new graph: %+v", index, err)
				return
			}

			// TODO: apply the global metaparams to the graph

			fastPause := false
			ge.Pause(fastPause) // sync
			if err := ge.Commit(); err != nil {
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: error running commit: %+v", index, err)
				return
			}
			if err := ge.Resume(); err != nil { // sync
				t.Errorf("test #%d: FAIL", index)
				t.Errorf("test #%d: error resuming graph: %+v", index, err)
				return
			}

			// wait for converger instead...
			select {
			case <-converged:
			case <-time.After(5 * time.Second): // temporary

				// XXX: add this when we debug converger
				//case <-time.After(60 * time.Second): // blocked or non-converged engine?
				//	t.Errorf("test #%d: FAIL", index)
				//	t.Errorf("test #%d: stream timeout", index)
				//	return
			}

			ngraph := ge.Graph()

			t.Logf("test #%d: graph: %+v", index, ngraph)
			str := strings.Trim(ngraph.Sprint(), "\n") // text format of output graph

			for i, v := range ngraph.Vertices() {
				res, ok := v.(engine.Res)
				if !ok {
					t.Errorf("test #%d: FAIL\n\n", index)
					t.Logf("test #%d: unexpected non-resource: %+v", index, v)
					return
				}
				m, err := engineUtil.ResToParamValues(res)
				if err != nil {
					t.Errorf("test #%d: FAIL\n\n", index)
					t.Logf("test #%d: can't read resource: %+v", index, err)
					return
				}
				if i == 0 {
					str += "\n"
				}
				keys := []string{}
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys) // sort for determinism
				for _, field := range keys {
					v := m[field]
					str += fmt.Sprintf("Field: %s[%s].%s = %s\n", res.Kind(), res.Name(), field, v)
				}
				if i < len(ngraph.Vertices()) {
					str += "\n"
				}
			}

			if expstr == magicEmpty {
				expstr = ""
			}
			// XXX: something isn't consistent, and I can't figure
			// out what, so workaround this by sorting these :(
			sortHack := func(x string) string {
				l := strings.Split(strings.TrimSpace(x), "\n")
				sort.Strings(l)
				return strings.TrimSpace(strings.Join(l, "\n"))
			}
			str = sortHack(str)
			expstr = sortHack(expstr)
			if expstr != str {
				t.Errorf("test #%d: FAIL\n\n", index)
				t.Logf("test #%d:   actual (g1):\n%s\n\n", index, str)
				t.Logf("test #%d: expected (g2):\n%s\n\n", index, expstr)
				diff := pretty.Compare(str, expstr)
				if diff != "" { // bonus
					t.Logf("test #%d: diff:\n%s", index, diff)
				}
				return
			}

			for i, v := range ngraph.Vertices() {
				t.Logf("test #%d: vertex(%d): %+v", index, i, v)
			}
			for v1 := range ngraph.Adjacency() {
				for v2, e := range ngraph.Adjacency()[v1] {
					t.Logf("test #%d: edge(%+v): %+v -> %+v", index, e, v1, v2)
				}
			}

			if !t.Failed() {
				t.Logf("test #%d: Passed!", index)
			}
		})
	}
	if testing.Short() {
		t.Skip("skipping all tests...")
	}
}

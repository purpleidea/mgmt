// Mgmt
// Copyright (C) James Shubin and the project contributors
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

package docs

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"

	cliUtil "github.com/purpleidea/mgmt/cli/util"
	docsUtil "github.com/purpleidea/mgmt/docs/util"
	"github.com/purpleidea/mgmt/engine"
	engineUtil "github.com/purpleidea/mgmt/engine/util"
	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/util"
)

const (
	// JSONSuffix is the output extension for the generated documentation.
	JSONSuffix = ".json"
)

// Generate is the main entrypoint for this command. It generates everything.
type Generate struct {
	*cliUtil.DocsGenerateArgs // embedded config
	Config                    // embedded Config

	// Program is the name of this program, usually set at compile time.
	Program string

	// Version is the version of this program, usually set at compile time.
	Version string

	// Debug represents if we're running in debug mode or not.
	Debug bool

	// Logf is a logger which should be used.
	Logf func(format string, v ...interface{})
}

// Main runs everything for this setup item.
func (obj *Generate) Main(ctx context.Context) error {
	if err := obj.Validate(); err != nil {
		return err
	}

	if err := obj.Run(ctx); err != nil {
		return err
	}

	return nil
}

// Validate verifies that the structure has acceptable data stored within.
func (obj *Generate) Validate() error {
	if obj == nil {
		return fmt.Errorf("data is nil")
	}
	if obj.Program == "" {
		return fmt.Errorf("program is empty")
	}
	if obj.Version == "" {
		return fmt.Errorf("version is empty")
	}

	return nil
}

// Run performs the desired actions to generate the documentation.
func (obj *Generate) Run(ctx context.Context) error {

	outputFile := obj.DocsGenerateArgs.Output
	if outputFile == "" || !strings.HasSuffix(outputFile, JSONSuffix) {
		return fmt.Errorf("must specify output")
	}
	// support relative paths too!
	if !strings.HasPrefix(outputFile, "/") {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		outputFile = filepath.Join(wd, outputFile)
	}

	if obj.Debug {
		obj.Logf("output: %s", outputFile)
	}

	// Ensure the directory exists.
	//d := filepath.Dir(outputFile)
	//if err := os.MkdirAll(d, 0750); err != nil {
	//	return fmt.Errorf("could not make output dir at: %s", d)
	//}

	resources, err := obj.genResources()
	if err != nil {
		return err
	}

	functions, err := obj.genFunctions()
	if err != nil {
		return err
	}

	data := &Output{
		Version:   safeVersion(obj.Version),
		Resources: resources,
		Functions: functions,
	}

	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	b = append(b, '\n') // needs a trailing newline

	if err := os.WriteFile(outputFile, b, 0600); err != nil {
		return err
	}
	obj.Logf("wrote: %s", outputFile)

	return nil
}

func (obj *Generate) getResourceInfo(kind, filename, structName string) (*ResourceInfo, error) {
	rootDir := obj.DocsGenerateArgs.RootDir
	if rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		rootDir = wd + "/" // add a trailing slash
	}
	if !strings.HasPrefix(rootDir, "/") || !strings.HasSuffix(rootDir, "/") {
		return nil, fmt.Errorf("bad root dir: %s", rootDir)
	}

	// filename might be "noop.go" for example
	p := filepath.Join(rootDir, engine.ResourcesRelDir, filename)

	fset := token.NewFileSet()

	// f is a: https://golang.org/pkg/go/ast/#File
	f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	// mcl field name to golang field name
	mapping, err := engineUtil.LangFieldNameToStructFieldName(kind)
	if err != nil {
		return nil, err
	}
	// golang field name to mcl field name
	nameMap, err := util.MapSwap(mapping)
	if err != nil {
		return nil, err
	}
	// mcl field name to mcl type
	typMap, err := engineUtil.LangFieldNameToStructType(kind)
	if err != nil {
		return nil, err
	}

	ri := &ResourceInfo{}
	// Populate the fields, even if they don't have a comment.
	ri.Name = structName // golang name
	ri.Kind = kind       // duplicate data
	ri.File = filename
	ri.Fields = make(map[string]*ResourceFieldInfo)
	for mclFieldName, fieldName := range mapping {
		typ, exists := typMap[mclFieldName]
		if !exists {
			continue
		}

		ri.Fields[mclFieldName] = &ResourceFieldInfo{
			Name: fieldName,
			Type: typ.String(),
			Desc: "", // empty for now
		}
	}

	var previousComment *ast.CommentGroup

	// Walk through the AST...
	ast.Inspect(f, func(node ast.Node) bool {

		// Comments above the struct appear as a node right _before_ we
		// find the struct, so if we see one, save it for later...
		if cg, ok := node.(*ast.CommentGroup); ok {
			previousComment = cg
			return true
		}

		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		name := typeSpec.Name.Name // name is now known!

		// If the struct isn't what we're expecting, then move on...
		if name != structName {
			return true
		}

		// Check if the TypeSpec is a named struct type that we want...
		st, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			return true
		}

		// At this point, we have the struct we want...

		var comment *ast.CommentGroup
		if typeSpec.Doc != nil {
			// I don't know how to even get here...
			comment = typeSpec.Doc // found!

		} else if previousComment != nil {
			comment = previousComment // found!
			previousComment = nil
		}

		ri.Desc = commentCleaner(comment)

		// Iterate over the fields of the struct
		for _, field := range st.Fields.List {
			// Check if the field has a comment associated with it
			if field.Doc == nil {
				continue
			}

			if len(field.Names) < 1 { // XXX: why does this happen?
				continue
			}

			fieldName := field.Names[0].Name
			if fieldName == "" { // Can this happen?
				continue
			}
			if isPrivate(fieldName) {
				continue
			}

			mclFieldName, exists := nameMap[fieldName]
			if !exists {
				continue
			}

			ri.Fields[mclFieldName].Desc = commentCleaner(field.Doc)
		}

		return true
	})

	return ri, nil
}

func (obj *Generate) genResources() (map[string]*ResourceInfo, error) {
	resources := make(map[string]*ResourceInfo)
	if obj.DocsGenerateArgs.NoResources {
		return resources, nil
	}

	r := engine.RegisteredResourcesNames()
	sort.Strings(r)
	for _, kind := range r {
		metadata, err := docsUtil.LookupResource(kind)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(kind, "_") {
			// TODO: Should we display these somehow?
			// built-in resource
			continue
		}

		ri, err := obj.getResourceInfo(kind, metadata.Filename, metadata.Typename)
		if err != nil {
			return nil, err
		}

		if ri.Name == "" {
			return nil, fmt.Errorf("empty resource name: %s", kind)
		}
		if ri.File == "" {
			return nil, fmt.Errorf("empty resource file: %s", kind)
		}
		if ri.Desc == "" {
			obj.Logf("empty resource desc: %s", kind)
		}
		fields := []string{}
		for field := range ri.Fields {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			if ri.Fields[field].Desc == "" {
				obj.Logf("empty resource (%s) field desc: %s", kind, field)
			}
		}

		resources[kind] = ri
	}

	return resources, nil
}

func (obj *Generate) getFunctionInfo(pkg, name string, metadata *docsUtil.Metadata) (*FunctionInfo, error) {
	rootDir := obj.DocsGenerateArgs.RootDir
	if rootDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		rootDir = wd + "/" // add a trailing slash
	}
	if !strings.HasPrefix(rootDir, "/") || !strings.HasSuffix(rootDir, "/") {
		return nil, fmt.Errorf("bad root dir: %s", rootDir)
	}
	if metadata.Filename == "" {
		return nil, fmt.Errorf("empty filename for: %s.%s", pkg, name)
	}

	// filename might be "pow.go" for example and contain a rel dir
	p := filepath.Join(rootDir, funcs.FunctionsRelDir, metadata.Filename)

	fset := token.NewFileSet()

	// f is a: https://golang.org/pkg/go/ast/#File
	f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	fi := &FunctionInfo{}
	fi.Name = metadata.Typename
	fi.File = metadata.Filename

	var previousComment *ast.CommentGroup
	found := false

	rawFunc := func(node ast.Node) (*ast.CommentGroup, string) {
		fd, ok := node.(*ast.FuncDecl)
		if !ok {
			return nil, ""
		}
		return fd.Doc, fd.Name.Name // name is now known!
	}

	rawStruct := func(node ast.Node) (*ast.CommentGroup, string) {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok {
			return nil, ""
		}

		// Check if the TypeSpec is a named struct type that we want...
		if _, ok := typeSpec.Type.(*ast.StructType); !ok {
			return nil, ""
		}

		return typeSpec.Doc, typeSpec.Name.Name // name is now known!
	}

	// Walk through the AST...
	ast.Inspect(f, func(node ast.Node) bool {

		// Comments above the struct appear as a node right _before_ we
		// find the struct, so if we see one, save it for later...
		if cg, ok := node.(*ast.CommentGroup); ok {
			previousComment = cg
			return true
		}

		doc, name := rawFunc(node) // First see if it's a raw func.
		if name == "" {
			doc, name = rawStruct(node) // Otherwise it's a struct.
		}

		// If the func isn't what we're expecting, then move on...
		if name != metadata.Typename {
			return true
		}

		var comment *ast.CommentGroup
		if doc != nil {
			// I don't know how to even get here...
			comment = doc // found!

		} else if previousComment != nil {
			comment = previousComment // found!
			previousComment = nil
		}

		fi.Desc = commentCleaner(comment)
		found = true

		return true
	})

	if !found {
		//return nil, nil
	}

	return fi, nil
}

func (obj *Generate) genFunctions() (map[string]*FunctionInfo, error) {
	functions := make(map[string]*FunctionInfo)
	if obj.DocsGenerateArgs.NoFunctions {
		return functions, nil
	}

	m := funcs.Map() // map[string]func() interfaces.Func
	names := []string{}
	for name := range m {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		a := names[i]
		b := names[j]
		// TODO: do a sorted-by-package order.
		return a < b
	})

	for _, name := range names {
		//v := m[name]
		//fn := v()
		fn := m[name]()

		// eg: golang/strings.has_suffix
		sp := strings.Split(name, ".")
		if len(sp) == 0 {
			return nil, fmt.Errorf("unexpected empty function")
		}
		if len(sp) > 2 {
			return nil, fmt.Errorf("unexpected function name: %s", name)
		}
		n := sp[0]
		p := sp[0]
		if len(sp) == 1 { // built-in
			p = "" // no package!
		}
		if len(sp) == 2 { // normal import
			n = sp[1]
		}

		if strings.HasPrefix(n, "_") {
			// TODO: Should we display these somehow?
			// built-in function
			continue
		}

		var sig *string
		//iface := ""
		if x := fn.Info().Sig; x != nil {
			s := x.String()
			sig = &s
			//iface = "simple"
		}

		metadata := &docsUtil.Metadata{}

		// XXX: maybe we need a better way to get this?
		mdFunc, ok := fn.(interfaces.MetadataFunc)
		if !ok {
			// Function doesn't tell us what the data is, let's try
			// to get it automatically...
			metadata.Typename = funcs.GetFunctionName(fn) // works!
			metadata.Filename = ""                        // XXX: How can we get this?

			// XXX: We only need this back-channel metadata store
			// because we don't know how to get the filename without
			// manually writing code in each function. Alternatively
			// we could add a New() method to each struct and then
			// we could modify the struct instead of having it be
			// behind a copy which is needed to get new copies!
			var err error
			metadata, err = docsUtil.LookupFunction(name)
			if err != nil {
				return nil, err
			}

		} else if mdFunc == nil {
			// programming error
			return nil, fmt.Errorf("unexpected empty metadata for function: %s", name)

		} else {
			metadata = mdFunc.GetMetadata()
		}

		if metadata == nil {
			return nil, fmt.Errorf("unexpected nil metadata for function: %s", name)
		}

		// This may be an empty func name if the function did not know
		// how to get it. (This is normal for automatic regular funcs.)
		if metadata.Typename == "" {
			metadata.Typename = funcs.GetFunctionName(fn) // works!
		}

		fi, err := obj.getFunctionInfo(p, n, metadata)
		if err != nil {
			return nil, err
		}
		// We may not get any fields added if we can't find anything...
		fi.Name = metadata.Typename
		fi.Package = p
		fi.Func = n
		fi.File = metadata.Filename
		//fi.Desc = desc
		fi.Signature = sig

		// Hack for golang generated functions!
		if strings.HasPrefix(fi.Package, "golang/") && fi.File == "generated_funcs.go" {
			pkg := fi.Package[len("golang/"):]
			frag := strings.TrimPrefix(fi.Name, strings.Title(strings.Join(strings.Split(pkg, "/"), ""))) // yuck
			fi.File = fmt.Sprintf("https://pkg.go.dev/%s#%s", pkg, frag)
		}

		if fi.Func == "" {
			return nil, fmt.Errorf("empty function name: %s", name)
		}
		if fi.File == "" {
			return nil, fmt.Errorf("empty function file: %s", name)
		}
		if fi.Desc == "" {
			obj.Logf("empty function desc: %s", name)
		}
		if fi.Signature == nil {
			obj.Logf("empty function sig: %s", name)
		}

		functions[name] = fi
	}

	return functions, nil
}

// Output is the type of the final data that will be for the json output.
type Output struct {
	// Version is the sha1 or ref name of this specific version. This is
	// used if we want to generate documentation with links matching the
	// correct version. If unspecified then this assumes git master.
	Version string `json:"version"`

	// Resources contains the collection of every available resource!
	// FIXME: should this be a list instead?
	Resources map[string]*ResourceInfo `json:"resources"`

	// Functions contains the collection of every available function!
	// FIXME: should this be a list instead?
	Functions map[string]*FunctionInfo `json:"functions"`
}

// ResourceInfo stores some information about each resource.
type ResourceInfo struct {
	// Name is the golang name of this resource.
	Name string `json:"name"`

	// Kind is the kind of this resource.
	Kind string `json:"kind"`

	// File is the file name where this resource exists.
	File string `json:"file"`

	// Desc explains what this resource does.
	Desc string `json:"description"`

	// Fields is a collection of each resource field and corresponding info.
	Fields map[string]*ResourceFieldInfo `json:"fields"`
}

// ResourceFieldInfo stores some information about each field in each resource.
type ResourceFieldInfo struct {
	// Name is what this field is called in golang format.
	Name string `json:"name"`

	// Type is the mcl type for this field.
	Type string `json:"type"`

	// Desc explains what this field does.
	Desc string `json:"description"`
}

// FunctionInfo stores some information about each function.
type FunctionInfo struct {
	// Name is the golang name of this function. This may be an actual
	// function if used by the simple API, or the name of a struct.
	Name string `json:"name"`

	// Package is the import name to use to get to this function.
	Package string `json:"package"`

	// Func is the name of the function in that package.
	Func string `json:"func"`

	// File is the file name where this function exists.
	File string `json:"file"`

	// Desc explains what this function does.
	Desc string `json:"description"`

	// Signature is the type signature of this function. If empty then the
	// signature is not known statically and it may be polymorphic.
	Signature *string `json:"signature,omitempty"`
}

// commentCleaner takes a comment group and returns it as a clean string. It
// removes the spurious newlines and programmer-focused comments. If there are
// blank lines, it replaces them with a single newline. The idea is that the
// webpage formatter would replace the newline with a <br /> or similar. This
// code is a modified alternative of the ast.CommentGroup.Text() function.
func commentCleaner(g *ast.CommentGroup) string {
	if g == nil {
		return ""
	}
	comments := make([]string, len(g.List))
	for i, c := range g.List {
		comments[i] = c.Text
	}

	lines := make([]string, 0, 10) // most comments are less than 10 lines
	for _, c := range comments {
		// Remove comment markers.
		// The parser has given us exactly the comment text.
		switch c[1] {
		case '/':
			//-style comment (no newline at the end)
			c = c[2:]
			if len(c) == 0 {
				// empty line
				break
			}
			if isDevComment(c[1:]) { // get rid of one space
				continue
			}
			if c[0] == ' ' {
				// strip first space - required for Example tests
				c = c[1:]
				break
			}
			//if isDirective(c) {
			//	// Ignore //go:noinline, //line, and so on.
			//	continue
			//}
		case '*':
			/*-style comment */
			c = c[2 : len(c)-2]
		}

		// Split on newlines.
		cl := strings.Split(c, "\n")

		// Walk lines, stripping trailing white space and adding to list.
		for _, l := range cl {
			lines = append(lines, stripTrailingWhitespace(l))
		}
	}

	// Remove leading blank lines; convert runs of interior blank lines to a
	// single blank line.
	n := 0
	for _, line := range lines {
		if line != "" || n > 0 && lines[n-1] != "" {
			lines[n] = line
			n++
		}
	}
	lines = lines[0:n]

	// Concatenate all of these together. Blank lines should be a newline.
	s := ""
	for i, line := range lines {
		if line == "" {
			continue
		}
		s += line
		if i < len(lines)-1 { // Is there another line?
			if lines[i+1] == "" {
				s += "\n" // Will eventually be a line break.
			} else {
				s += " "
			}
		}
	}

	return s
}

// TODO: should we use unicode.IsSpace instead?
func isWhitespace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' }

// TODO: should we replace with a strings package stdlib function?
func stripTrailingWhitespace(s string) string {
	i := len(s)
	for i > 0 && isWhitespace(s[i-1]) {
		i--
	}
	return s[0:i]
}

// isPrivate specifies if a field name is "private" or not.
func isPrivate(fieldName string) bool {
	if fieldName == "" {
		panic("invalid field name")
	}
	x := fieldName[0:1]

	if strings.ToLower(x) == x {
		return true // it was already private
	}

	return false
}

// isDevComment tells us that the comment is for developers only!
func isDevComment(comment string) bool {
	if strings.HasPrefix(comment, "TODO:") {
		return true
	}
	if strings.HasPrefix(comment, "FIXME:") {
		return true
	}
	if strings.HasPrefix(comment, "XXX:") {
		return true
	}
	return false
}

// safeVersion parses the main version string and returns a short hash for us.
// For example, we might get a string of 0.0.26-176-gabcdef012-dirty as input,
// and we'd want to return abcdef012.
func safeVersion(version string) string {
	const dirty = "-dirty"

	s := version
	if strings.HasSuffix(s, dirty) { // helpful dirty remover
		s = s[0 : len(s)-len(dirty)]
	}

	ix := strings.LastIndex(s, "-")
	if ix == -1 { // assume we have a standalone version (future proofing?)
		return s
	}
	s = s[ix+1:]

	// From the `git describe` man page: The "g" prefix stands for "git" and
	// is used to allow describing the version of a software depending on
	// the SCM the software is managed with. This is useful in an
	// environment where people may use different SCMs.
	const g = "g"
	if strings.HasPrefix(s, g) {
		s = s[len(g):]
	}

	return s
}

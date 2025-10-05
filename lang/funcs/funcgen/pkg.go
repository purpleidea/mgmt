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

package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iancoleman/strcase"
	yaml "gopkg.in/yaml.v2"

	"golang.org/x/tools/go/packages"
)

type golangPackages []*golangPackage

type golangPackage struct {
	// Name is the name of the golang package.
	Name string `yaml:"name"`
	// Alias is the alias of the package when imported in golang.
	// e.g. import rand "os.rand"
	Alias string `yaml:"alias,omitempty"`
	// MgmtAlias is the name of the package inside mcl.
	MgmtAlias string `yaml:"mgmtAlias,omitempty"`
	// Exclude is a list of golang function names that we do not want.
	Exclude []string `yaml:"exclude,omitempty"`
}

func parsePkg(path, filename, templates string) error {
	var c config
	filePath := filepath.Join(path, filename)
	log.Printf("Data: %s", filePath)
	cfgFile, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	err = yaml.UnmarshalStrict(cfgFile, &c)
	if err != nil {
		return err
	}
	functions, err := parsePackages(c)
	if err != nil {
		return err
	}
	return parseFuncs(c, functions, path, templates)
}

func parsePackages(c config) (functions, error) {
	var funcs functions
	for _, golangPackage := range c.Packages {
		fn, err := golangPackage.parsefuncs()
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, fn...)
	}
	return funcs, nil
}

// parsefuncs discovers exported top-level functions for obj.Name using
// go/packages and go/types.
func (obj *golangPackage) parsefuncs() (functions, error) {
	pkg, err := loadPackage(obj.Name)
	if err != nil {
		return nil, err
	}

	// Gather doc comments by name.
	funcDocs := collectFuncDocs(pkg)

	// List exported, top-level functions from the types.Scope.
	scope := pkg.Types.Scope()
	var names []string
	for _, n := range scope.Names() {
		if token.IsExported(n) {
			if _, ok := scope.Lookup(n).(*types.Func); ok {
				names = append(names, n)
			}
		}
	}
	sort.Strings(names) // deterministic output

	// Build exclusion lookup.
	excl := make(map[string]struct{}, len(obj.Exclude))
	for _, e := range obj.Exclude {
		excl[e] = struct{}{}
	}

	var out functions
	for _, name := range names {
		if _, skip := excl[name]; skip {
			continue
		}
		o := scope.Lookup(name).(*types.Func)
		sig, ok := o.Type().(*types.Signature)
		if !ok || sig.Recv() != nil {
			continue // methods or unexpected
		}

		// Convert parameters/results according to our allowlist.
		fnArgs, ok := mapTuple(sig.Params(), paramPolicy)
		if !ok {
			continue
		}
		returns, errorFul, ok := mapResults(sig.Results())
		if !ok || len(returns) == 0 {
			continue
		}

		// Names used in generated code.
		mgmtPackage := obj.Name
		if obj.MgmtAlias != "" {
			mgmtPackage = obj.MgmtAlias
		}
		mgmtPackage = fmt.Sprintf("golang/%s", mgmtPackage)

		internalName := fmt.Sprintf("%s%s", strcase.ToCamel(strings.Replace(obj.Name, "/", "", -1)), name)
		internalName = strings.Replace(internalName, "Html", "HTML", -1)

		// Help/docstring: header + signature + doc.
		help := buildHelp(internalName, name, sig, funcDocs[name])

		out = append(out, &function{
			MgmtPackage:   mgmtPackage,
			MclName:       strcase.ToSnake(name),
			InternalName:  internalName,
			Help:          help,
			GolangPackage: obj,
			GolangFunc:    name,
			Errorful:      errorFul,
			Args:          fnArgs,
			Variadic:      sig.Variadic(),
			Return:        returns,
		})
	}
	return out, nil
}

type typePolicy uint8

const (
	paramPolicy  typePolicy = iota // parameters policy
	resultPolicy                   // results policy
)

// mapTuple converts a go/types.Tuple (the ordered list of parameters or results
// in a function signature) into our generator's []arg representation.
//
// For each element in the tuple, calls mapType with the given policy (param vs.
// result) and synthesizes an arg with a name (from the source, or argN
// fallback).
//
// Returns the full []arg slice if all element types are supported; ok=false if
// any element type is unsupported, in which case we skip generating this
// function entirely.
func mapTuple(tup *types.Tuple, policy typePolicy) ([]arg, bool) {
	args := make([]arg, 0, tup.Len())
	for i := 0; i < tup.Len(); i++ {
		v := tup.At(i)
		ts, ok := mapType(v.Type(), policy)
		if !ok {
			return nil, false
		}
		name := v.Name()
		if name == "" {
			name = fmt.Sprintf("arg%d", i)
		}
		args = append(args, arg{Name: name, Type: ts})
	}
	return args, true
}

// mapResults enforces the special shape rules we allow for return values.
//
// We only accept two cases:
// 1. Exactly one result: of a supported simple type (mapType with
// resultPolicy).
// 2. Exactly two results: (T, error) where T is a supported simple type and the
// second result is the error type.
//
// Anything else (0 results, >2 results, multiple non-error results, etc.) is
// rejected.
//
// Returns []arg representing the non-error return(s); a bool 'errorful' which
// is true if the function is (T, error); ok=false if the results don't match
// the allowed shapes.
func mapResults(tup *types.Tuple) ([]arg, bool, bool) {
	switch tup.Len() {
	case 1:
		if ts, ok := mapType(tup.At(0).Type(), resultPolicy); ok {
			return []arg{{Type: ts}}, false, true
		}
	case 2:
		if !isErrorType(tup.At(1).Type()) {
			return nil, false, false
		}
		if ts, ok := mapType(tup.At(0).Type(), resultPolicy); ok {
			return []arg{{Type: ts}}, true, true
		}
	}
	return nil, false, false
}

// mapType inspects a single Go type (from go/types) and decides if our
// generator/MCL runtime knows how to handle it.
//
// Returns the generator type string we use in templates ("bool", "string",
// "int64", "[]byte", "[]string", ...); a boolean ok flag (false means: type not
// supported, skip this function).
//
// While using paramPolicy parameters are more permissive. Both []byte and
// []string are accepted. The latter is used to model variadic ...string. With
// resultPolicy results are stricter: []byte is allowed (so the template can
// wrap it with string([]byte)), but []string is rejected (no runtime support).
func mapType(t types.Type, policy typePolicy) (string, bool) {
	switch bt := t.(type) {
	case *types.Basic:
		switch bt.Kind() {
		case types.Bool:
			return "bool", true
		case types.String:
			return "string", true
		case types.Int:
			return "int", true
		case types.Int64:
			return "int64", true
		case types.Float64:
			return "float64", true
		default:
			return "", false
		}

	case *types.Slice:
		// []byte and []string handling differs between params/results.
		if b, ok := bt.Elem().(*types.Basic); ok {
			switch b.Kind() {
			case types.Byte: // alias for uint8
				// Params: accept []byte; Results: keep as []byte (template adds string([]byte)).
				return "[]byte", true
			case types.String:
				// Params: accept []string (for variadics like ...string represented as []string).
				// Results: reject (no list value type at runtime).
				if policy == paramPolicy {
					return "[]string", true
				}
				return "", false
			}
		}
	}
	return "", false
}

func isErrorType(t types.Type) bool {
	if ni, ok := t.(*types.Named); ok {
		return ni.Obj().Name() == "error" && ni.Obj().Pkg() == nil
	}
	return false
}

// loadPackage resolves and loads a single package import path with types and
// syntax.
func loadPackage(importPath string) (*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedCompiledGoFiles |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedSyntax |
			packages.NeedModule,
		Env: os.Environ(), // allow both stdlib and external modules
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, err
	}
	if packages.PrintErrors(pkgs) > 0 {
		return nil, fmt.Errorf("failed to load %q", importPath)
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil || pkgs[0].Types.Scope() == nil {
		return nil, fmt.Errorf("no types for %q", importPath)
	}
	return pkgs[0], nil
}

// collectFuncDocs scans a package's AST and returns doc comments for top-level
// functions. Methods with receivers are ignored.
func collectFuncDocs(p *packages.Package) map[string]string {
	docs := make(map[string]string)
	for _, f := range p.Syntax {
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue // methods or invalid
			}
			if fn.Doc != nil {
				docs[fn.Name.Name] = strings.TrimRight(fn.Doc.Text(), "\n")
			}
		}
	}
	return docs
}

// buildHelp builds a help block: autogenerated header + signature + doc text.
func buildHelp(internalName, publicName string, sig *types.Signature, doc string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s is an autogenerated function.\n", internalName)

	// Signature line from go/types.
	s := types.TypeString(sig, nil) // includes "func("
	// Ensure it starts with "func Name(" rather than "func(".
	if strings.HasPrefix(s, "func(") {
		s = "func " + publicName + s[len("func"):]
	}
	fmt.Fprintf(&b, "// %s\n", s)

	if doc != "" {
		for _, line := range strings.Split(doc, "\n") {
			line = strings.TrimLeft(strings.TrimRight(line, "\r\n"), " \t")
			if line == "" {
				b.WriteString("//\n")
			} else {
				b.WriteString("// ")
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

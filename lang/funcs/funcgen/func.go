// Mgmt
// Copyright (C) 2013-2019+ James Shubin and the project contributors
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

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

type function struct {
	MgmtPackage string     `yaml:"mgmtPackage"`
	MgmtName    string     `yaml:"mgmtName"`
	Help        string     `yaml:"help"`
	GoPackage   string     `yaml:"goPackage"`
	GoFunc      string     `yaml:"goFunc"`
	Args        []arg      `yaml:"args"`
	Return      []arg      `yaml:"return"`
	Tests       []functest `yaml:"tests"`
}

type functest struct {
	Args   []testarg `yaml:"args"`
	Expect []testarg `yaml:"return"`
}

type templateInput struct {
	Func        function
	MgmtPackage string
}

func parseFuncs(c config, path, templates string) error {
	templateFiles, err := filepath.Glob(templates)
	if err != nil {
		return err
	}
	for _, tpl := range templateFiles {
		log.Printf("Template: %s", tpl)
		err = generateTemplate(c, path, tpl)
		if err != nil {
			return err
		}
	}
	return nil
}

func generateTemplate(c config, path, templateFile string) error {
	log.Printf("Reading: %s", templateFile)
	basename := filepath.Base(templateFile)
	tplFile, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return err
	}
	t, err := template.New(basename).Parse(string(tplFile))
	if err != nil {
		return err
	}
	finalName := strings.TrimSuffix(basename, ".tpl")
	finalPath := filepath.Join(path, finalName)
	log.Printf("Writing: %s", finalPath)
	finalFile, err := os.Create(finalPath)
	if err != nil {
		return err
	}
	if err = t.Execute(finalFile, c); err != nil {
		return err
	}
	return nil
}

// MakeGoArgs translates the func args to go args.
func (obj *function) MakeGoArgs() (string, error) {
	var args []string
	for i, a := range obj.Args {
		gol, err := a.ToGo()
		if err != nil {
			return "", err
		}
		args = append(args, fmt.Sprintf("input[%d].%s()", i, gol))
	}
	return strings.Join(args, ", "), nil
}

// Signature generates the mcl signature of the function.
func (obj *function) Signature() (string, error) {
	var args []string
	for _, a := range obj.Args {
		mcl, err := a.ToMcl()
		if err != nil {
			return "", err
		}
		args = append(args, mcl)
	}
	var returns []string
	for _, a := range obj.Return {
		mcl, err := a.ToMcl()
		if err != nil {
			return "", err
		}
		returns = append(returns, mcl)
	}
	return fmt.Sprintf("func(%s) %s", strings.Join(args, ", "), returns[0]), nil
}

// MakeGoReturn returns the golang signature of the return.
func (obj *function) MakeGoReturn() (string, error) {
	return obj.Return[0].ToGo()
}

// MakeGoTypeReturn returns the mcl signature of the return.
func (obj *function) MakeGoTypeReturn() string {
	return obj.Return[0].Type
}

// MakeTestSign returns the signature of the test.
func (obj *function) MakeTestSign() string {
	var args []string
	for i, a := range obj.Args {
		var nextSign string
		if i+1 < len(obj.Args) {
			nextSign = obj.Args[i+1].Type
		} else {
			nextSign = obj.MakeGoTypeReturn()
		}
		if nextSign == a.Type {
			args = append(args, a.Name)
		} else {
			args = append(args, fmt.Sprintf("%s %s", a.Name, a.Type))
		}
	}
	args = append(args, fmt.Sprintf("expected %s", obj.MakeGoTypeReturn()))
	return strings.Join(args, ", ")
}

// TestInput generated a string that can be passed as test input.
func (obj *function) TestInput() (string, error) {
	var values []string
	for _, i := range obj.Args {
		tti, err := i.ToTestInput()
		if err != nil {
			return "", err
		}
		values = append(values, tti)
	}
	return fmt.Sprintf("[]types.Value{%s}", strings.Join(values, ", ")), nil
}

// MakeTestArgs generates a string that can be passed a test arguments.
func (obj *functest) MakeTestArgs() string {
	var values []string
	for _, i := range obj.Args {
		if i.Type == "string" {
			values = append(values, fmt.Sprintf(`"%s"`, i.Value))
		}
	}
	for _, i := range obj.Expect {
		if i.Type == "string" {
			values = append(values, fmt.Sprintf(`"%s"`, i.Value))
		}
	}
	return strings.Join(values, ", ")
}

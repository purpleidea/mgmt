// Mgmt
// Copyright (C) 2013-2022+ James Shubin and the project contributors
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
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/iancoleman/strcase"
	yaml "gopkg.in/yaml.v2"
)

var (
	validSignature = regexp.MustCompile(`^func (?P<name>[A-Z][a-zA-Z0-9]+)\((?P<args>([a-zA-Z]+( (bool|string|int|int64|float64|\[\]byte))?(, )?){0,})\) (?P<return>(bool|string|int|int64|float64|\[\]byte|)|\((bool|string|int|int64|float64|\[\]byte), error\))$`)
	errExcluded    = errors.New("function is excluded")
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
	cfgFile, err := ioutil.ReadFile(filePath)
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
	err = parseFuncs(c, functions, path, templates)
	if err != nil {
		return err
	}
	return nil
}

func parsePackages(c config) (functions, error) {
	var funcs []function
	for _, golangPackage := range c.Packages {
		fn, err := golangPackage.parsefuncs()
		if err != nil {
			return funcs, err
		}
		funcs = append(funcs, fn...)
	}
	return funcs, nil
}

func (obj *golangPackage) parsefuncs() (functions, error) {
	var funcs []function
	cmd := exec.Command("go", "doc", obj.Name)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return funcs, err
	}
	return obj.extractFuncs(out.String(), true)
}

func (obj *golangPackage) extractFuncs(doc string, getHelp bool) (functions, error) {
	var funcs []function
	for _, line := range strings.Split(doc, "\n") {
		if validSignature.MatchString(line) {
			f, err := obj.parseFunctionLine(line, getHelp)
			if err != nil && err != errExcluded {
				return funcs, err
			}
			if f != nil {
				funcs = append(funcs, *f)
			}
		}
	}

	return funcs, nil
}

func (obj *golangPackage) parseFunctionLine(line string, getHelp bool) (*function, error) {
	match := validSignature.FindStringSubmatch(line)
	result := make(map[string]string)
	for i, name := range validSignature.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}

	name := result["name"]

	for _, e := range obj.Exclude {
		if e == name {
			return nil, errExcluded
		}
	}

	errorFul, err := regexp.MatchString(`, error\)$`, result["return"])
	if err != nil {
		return nil, err
	}

	returns := parseReturn(result["return"])
	if len(returns) == 0 {
		return nil, errExcluded
	}

	mgmtPackage := obj.Name
	if obj.MgmtAlias != "" {
		mgmtPackage = obj.MgmtAlias
	}
	mgmtPackage = fmt.Sprintf("golang/%s", mgmtPackage)

	internalName := fmt.Sprintf("%s%s", strcase.ToCamel(strings.Replace(obj.Name, "/", "", -1)), name)
	internalName = strings.Replace(internalName, "Html", "HTML", -1)
	var help string
	if getHelp {
		help, err = obj.getHelp(name, internalName)
		if err != nil {
			return nil, err
		}
	}

	return &function{
		MgmtPackage:   mgmtPackage,
		MclName:       strcase.ToSnake(name),
		InternalName:  internalName,
		Help:          help,
		GolangPackage: obj,
		GolangFunc:    name,
		Errorful:      errorFul,
		Args:          parseArgs(result["args"]),
		Return:        returns,
	}, nil
}

func reverseArgs(s []arg) {
	last := len(s) - 1
	for i := 0; i < len(s)/2; i++ {
		s[i], s[last-i] = s[last-i], s[i]
	}
}
func reverse(s []string) {
	last := len(s) - 1
	for i := 0; i < len(s)/2; i++ {
		s[i], s[last-i] = s[last-i], s[i]
	}
}

func parseArgs(str string) []arg {
	var args []arg
	s := strings.Split(str, ",")
	reverse(s)
	var currentType string
	for _, currentArg := range s {
		if currentArg == "" {
			continue
		}
		v := strings.Split(strings.TrimSpace(currentArg), " ")
		if len(v) == 2 {
			currentType = v[1]
		}
		args = append(args, arg{Name: v[0], Type: currentType})
	}
	reverseArgs(args)
	return args
}

func parseReturn(str string) []arg {
	var returns []arg
	re := regexp.MustCompile(`(bool|string|int|int64|float64|\[\]byte)`)
	t := string(re.Find([]byte(str)))
	returns = append(returns, arg{Type: t})
	return returns
}

func (obj *golangPackage) getHelp(function, internalName string) (string, error) {
	cmd := exec.Command("go", "doc", fmt.Sprintf("%s.%s", obj.Name, function))
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	var doc string
	for i, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if i > 0 {
			s := strings.TrimSpace(line)
			if i == 1 {
				docs := strings.Split(s, " ")
				docs[0] = internalName
				s = strings.Join(docs, " ")
				doc = doc + "// " + s + " is an autogenerated function.\n"
			} else {
				doc = doc + "//"
				if s != "" {
					doc = doc + " "
				}
				doc = doc + s + "\n"
			}
		}
	}
	return doc, nil
}

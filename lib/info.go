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

package lib

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/purpleidea/mgmt/lang/funcs"
	"github.com/purpleidea/mgmt/lang/interfaces"
	"github.com/purpleidea/mgmt/resources"
	"github.com/urfave/cli"
)

const (
	twMinWidth = 0
	twTabWidth = 8
	twPadding  = 2   // ensure columns have at least a space between them
	twPadChar  = ' ' // using a tab here creates 'jumpy' columns on output
	twFlags    = 0
)

// info wraps infocmd to produce output to stdout
func info(c *cli.Context) error {
	output, err := infoCmd(c)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, twMinWidth, twTabWidth, twPadding, twPadChar, twFlags)
	fmt.Fprint(w, output.String())
	w.Flush()
	return nil
}

// infoCmd takes the cli context and returns the requested output
func infoCmd(c *cli.Context) (bytes.Buffer, error) {
	var out bytes.Buffer
	var names []string
	descriptions := make(map[string]string)

	switch c.Command.Name {
	case "resources":
		for _, name := range resources.RegisteredResourcesNames() {
			names = append(names, name)

			descriptions[name] = ""

			res, err := resources.Lookup(name)
			if err != nil {
				continue
			}

			s := reflect.ValueOf(res).Elem()
			typeOfT := s.Type()

			var fields []string
			for i := 0; i < s.NumField(); i++ {
				field := typeOfT.Field(i)
				// skip unexported fields
				if field.PkgPath != "" {
					continue
				}
				fieldname := field.Name
				if fieldname == "BaseRes" {
					continue
				}
				f := s.Field(i)
				fieldtype := f.Type()
				fields = append(fields, fmt.Sprintf("%s (%s)", strings.ToLower(fieldname), fieldtype))
			}
			descriptions[name] = strings.Join(fields, ", ")

		}
	case "functions":
		for name := range funcs.RegisteredFuncs {
			// skip internal functions (history, operations, etc)
			if strings.HasPrefix(name, "_") {
				continue
			}
			names = append(names, name)
			descriptions[name] = ""
			fn, err := funcs.Lookup(name)
			if err != nil {
				continue
			}
			if _, ok := fn.(interfaces.PolyFunc); !ok {
				// TODO: skip for now, needs Build before Info
				continue
			}

			// set function signature as description
			descriptions[name] = strings.Replace(fn.Info().Sig.String(), "func", name, 1)
		}
	default:
		return out, fmt.Errorf("invalid command")
	}

	sort.Strings(names)
	for _, name := range names {
		if c.Bool("type") {
			fmt.Fprintf(&out, "%s\t%s\n", name, descriptions[name])
		} else {
			fmt.Fprintln(&out, name)
		}
	}
	return out, nil
}

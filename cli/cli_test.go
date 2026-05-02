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

package cli

import (
	"reflect"
	"testing"

	"github.com/alexflint/go-arg"
	_ "github.com/purpleidea/mgmt/gapi/empty" // import so the gapi registers
)

func TestRunArgsPprof(t *testing.T) {
	args := &RunArgs{}
	parser, err := arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	if err := parser.Parse(NormalizeArgs([]string{"--pprof"})); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if args.Pprof == nil {
		t.Fatalf("Pprof is nil")
	}
	if *args.Pprof != "" {
		t.Fatalf("unexpected Pprof value: %s", *args.Pprof)
	}

	args = &RunArgs{}
	parser, err = arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	if err := parser.Parse([]string{"--pprof", "127.0.0.1:7000"}); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if args.Pprof == nil {
		t.Fatalf("Pprof is nil")
	}
	if *args.Pprof != "127.0.0.1:7000" {
		t.Fatalf("unexpected Pprof value: %s", *args.Pprof)
	}

	args = &RunArgs{}
	parser, err = arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("NewParser failed: %v", err)
	}
	if err := parser.Parse([]string{}); err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if args.Pprof != nil {
		t.Fatalf("Pprof is not nil: %s", *args.Pprof)
	}
}

func TestNormalizeArgs(t *testing.T) {
	testCases := []struct {
		name string
		in   []string
		out  []string
	}{
		{
			name: "default at end",
			in:   []string{"--pprof"},
			out:  []string{"--pprof", ""},
		},
		{
			name: "default before frontend",
			in:   []string{"--pprof", "empty"},
			out:  []string{"--pprof", "", "empty"},
		},
		{
			name: "default before flag",
			in:   []string{"--pprof", "--noop"},
			out:  []string{"--pprof", "", "--noop"},
		},
		{
			name: "empty equals",
			in:   []string{"--pprof="},
			out:  []string{"--pprof", ""},
		},
		{
			name: "custom value",
			in:   []string{"--pprof", "127.0.0.1:7000"},
			out:  []string{"--pprof", "127.0.0.1:7000"},
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			if result := NormalizeArgs(test.in); !reflect.DeepEqual(result, test.out) {
				t.Fatalf("unexpected args: %#v", result)
			}
		})
	}
}

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

//go:build !root

package cli

import (
	"reflect"
	"testing"

	"github.com/alexflint/go-arg"
	_ "github.com/purpleidea/mgmt/gapi/empty" // import so the gapi registers
)

func TestFmtArgs(t *testing.T) {
	args := &Args{}
	parser, err := arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("func NewParser failed: %v", err)
	}
	if err := parser.Parse([]string{"fmt", "examples/lang/hello0.mcl"}); err != nil {
		t.Fatalf("func Parse failed: %v", err)
	}
	if args.FmtCmd == nil {
		t.Fatalf("func FmtCmd is nil")
	}
	if args.FmtCmd.Test {
		t.Fatalf("func Test is true")
	}
	if args.FmtCmd.Input != "examples/lang/hello0.mcl" {
		t.Fatalf("unexpected input: %s", args.FmtCmd.Input)
	}
}

func TestCheckLangArgs(t *testing.T) {
	args := &Args{}
	parser, err := arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("func NewParser failed: %v", err)
	}
	argv := []string{
		"check",
		"lang",
		"--download",
		"--update",
		"--skip-unify",
		"--unify-name",
		"noop",
		"--unify-optimizations",
		"foo,bar",
		"--skip-fmt",
		"--depth",
		"42",
		"--retry",
		"3",
		"--module-path",
		"/tmp/modules/",
		"examples/lang/hello0.mcl",
	}
	if err := parser.Parse(argv); err != nil {
		t.Fatalf("func Parse failed: %v", err)
	}
	if args.CheckCmd == nil {
		t.Fatalf("func CheckCmd is nil")
	}
	if args.CheckCmd.CheckLang == nil {
		t.Fatalf("func CheckLang is nil")
	}

	cmd := args.CheckCmd.CheckLang
	if cmd.Input != "examples/lang/hello0.mcl" {
		t.Fatalf("unexpected input: %s", cmd.Input)
	}
	if !cmd.Download {
		t.Fatalf("download is false")
	}
	if !cmd.Update {
		t.Fatalf("update is false")
	}
	if !cmd.SkipUnify {
		t.Fatalf("skip unify is false")
	}
	if cmd.UnifySolver == nil || *cmd.UnifySolver != "noop" {
		t.Fatalf("unexpected unify solver: %#v", cmd.UnifySolver)
	}
	if len(cmd.UnifyOptimizations) != 1 || cmd.UnifyOptimizations[0] != "foo,bar" {
		t.Fatalf("unexpected unify optimizations: %#v", cmd.UnifyOptimizations)
	}
	if !cmd.SkipFmt {
		t.Fatalf("skip fmt is false")
	}
	if cmd.Depth != 42 {
		t.Fatalf("unexpected depth: %d", cmd.Depth)
	}
	if cmd.Retry != 3 {
		t.Fatalf("unexpected retry: %d", cmd.Retry)
	}
	if cmd.ModulePath != "/tmp/modules/" {
		t.Fatalf("unexpected module path: %s", cmd.ModulePath)
	}
}

func TestRunArgsPprof(t *testing.T) {
	args := &RunArgs{}
	parser, err := arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("func NewParser failed: %v", err)
	}
	if err := parser.Parse(NormalizeArgs([]string{"--pprof"})); err != nil {
		t.Fatalf("func Parse failed: %v", err)
	}
	if args.Pprof == nil {
		t.Fatalf("pprof is nil")
	}
	if *args.Pprof != "" {
		t.Fatalf("unexpected Pprof value: %s", *args.Pprof)
	}

	args = &RunArgs{}
	parser, err = arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("func NewParser failed: %v", err)
	}
	if err := parser.Parse([]string{"--pprof", "127.0.0.1:7000"}); err != nil {
		t.Fatalf("func Parse failed: %v", err)
	}
	if args.Pprof == nil {
		t.Fatalf("pprof is nil")
	}
	if *args.Pprof != "127.0.0.1:7000" {
		t.Fatalf("unexpected Pprof value: %s", *args.Pprof)
	}

	args = &RunArgs{}
	parser, err = arg.NewParser(arg.Config{}, args)
	if err != nil {
		t.Fatalf("func NewParser failed: %v", err)
	}
	if err := parser.Parse([]string{}); err != nil {
		t.Fatalf("func Parse failed: %v", err)
	}
	if args.Pprof != nil {
		t.Fatalf("pprof is not nil: %s", *args.Pprof)
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

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
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli"
)

func TestResources(t *testing.T) {
	set := flag.NewFlagSet("test", 0)
	ctx := cli.NewContext(nil, set, nil)
	ctx.Command = cli.Command{Name: "resources"}

	out, err := infoCmd(ctx)
	if err != nil {
		t.Fatal("failed")
	}
	assert.Contains(t, out.String(), "file")
}

func TestFunctionsWithTypes(t *testing.T) {
	set := flag.NewFlagSet("test", 0)
	set.Bool("type", true, "doc")
	ctx := cli.NewContext(nil, set, nil)
	ctx.Command = cli.Command{Name: "functions"}

	out, err := infoCmd(ctx)
	if err != nil {
		t.Fatal("failed")
	}
	assert.Contains(t, out.String(), "load() struct{x1 float; x5 float; x15 float}")
}

// TODO: see infoCmd(), this still needs some work
// func TestPolyFunctionsWithTypes(t *testing.T) {
// 	set := flag.NewFlagSet("test", 0)
// 	set.Bool("type", true, "doc")
// 	ctx := cli.NewContext(nil, set, nil)
// 	ctx.Command = cli.Command{Name: "functions"}
//
// 	out, err := infoCmd(ctx)
// 	if err != nil {
// 		t.Fatal("failed")
// 	}
// 	assert.Contains(t, out.String(), "somethingmore")
// }

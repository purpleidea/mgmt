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

//go:build !darwin

package coresys

import (
	"context"
	"testing"

	"github.com/purpleidea/mgmt/lang/funcs/facts"
	"github.com/purpleidea/mgmt/lang/types"
)

func TestSimple(t *testing.T) {
	fact := &CPUCountFact{}

	output := make(chan types.Value)
	err := fact.Init(&facts.Init{
		Output: output,
		Logf: func(format string, v ...interface{}) {
			t.Logf("cpucount_fact_test: "+format, v...)
		},
	})
	if err != nil {
		t.Errorf("could not init CPUCountFact")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer cancel()
	Loop:
		for {
			select {
			case cpus := <-output:
				t.Logf("CPUS: %d\n", cpus.Int())
				break Loop
			}
		}
	}()

	// now start the stream
	if err := fact.Stream(ctx); err != nil {
		t.Error(err)
	}
}

func TestParseCPUList(t *testing.T) {
	var cpulistTests = []struct {
		desc   string
		list   string
		result int64
	}{
		{
			desc:   "single CPU",
			list:   "1",
			result: 1,
		},
		{
			desc:   "cpu range",
			list:   "0-31",
			result: 32,
		},
		{
			desc:   "range and single",
			list:   "0-1,3",
			result: 3,
		},
		{
			desc:   "single, two ranges",
			list:   "2,4-8,10-16",
			result: 13,
		},
	}

	for _, tt := range cpulistTests {
		t.Run(tt.list, func(t *testing.T) {
			cpuCount, err := parseCPUList(tt.list)
			if err != nil {
				t.Errorf("could not parseCPUList: %+v", err)
				return
			}
			if cpuCount != tt.result {
				t.Errorf("expected %d, got %d", tt.result, cpuCount)
				return
			}
		})
	}
}

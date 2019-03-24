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

// +build !darwin

package coresys

import (
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

	go func() {
		defer fact.Close()
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
	if err := fact.Stream(); err != nil {
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

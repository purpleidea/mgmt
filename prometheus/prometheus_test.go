// Mgmt
// Copyright (C) 2013-2024+ James Shubin and the project contributors
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

package prometheus

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestInitKindMetrics tests that we are initializing the Prometheus metrics
// correctly for all kinds of resources.
func TestInitKindMetrics(t *testing.T) {
	var prom Prometheus
	prom.Init()
	prom.InitKindMetrics([]string{"file", "exec"})

	// Get a list of metrics collected by Prometheus.
	// This is the only way to get Prometheus metrics
	// without implicitly creating them.
	gatherer := prometheus.DefaultGatherer
	metrics, err := gatherer.Gather()
	if err != nil {
		t.Errorf("error while gathering metrics: %s", err)
		return
	}

	// expectedMetrics is a map: keys are metrics name and
	// values are expected and actual count of metrics with
	// that name.
	expectedMetrics := map[string][2]int{
		"mgmt_checkapply_total": {
			16, 0,
		},
		"mgmt_failures_total": {
			4, 0,
		},
		"mgmt_failures": {
			4, 0,
		},
		"mgmt_resources": {
			2, 0,
		},
	}

	for _, metric := range metrics {
		for name, count := range expectedMetrics {
			if *metric.Name == name {
				value := len(metric.Metric)
				expectedMetrics[name] = [2]int{count[0], value}
			}
		}
	}

	for name, count := range expectedMetrics {
		if count[1] != count[0] {
			t.Errorf("with: %s, expected %d metrics, got %d metrics", name, count[0], count[1])
		}
	}
}

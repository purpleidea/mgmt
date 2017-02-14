// Mgmt
// Copyright (C) 2013-2016+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// Package prometheus provides functions that are useful to control and manage
// the build-in prometheus instance.
package prometheus

import (
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DefaultPrometheusListen is registered in
// https://github.com/prometheus/prometheus/wiki/Default-port-allocations
const DefaultPrometheusListen = "127.0.0.1:9233"

// Prometheus is the struct that contains information about the
// prometheus instance. Run Init() on it.
type Prometheus struct {
	Listen string // the listen specification for the net/http server

	checkApplyTotal         *prometheus.CounterVec // total of CheckApplies that have been triggered
	processStartTimeSeconds prometheus.Gauge       // process start time in seconds since unix epoch

}

// Init some parameters - currently the Listen address.
func (obj *Prometheus) Init() error {
	if len(obj.Listen) == 0 {
		obj.Listen = DefaultPrometheusListen
	}
	obj.checkApplyTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_checkapply_total",
			Help: "Number of CheckApply that have run.",
		},
		// Labels for this metric.
		// kind: resource type: Svc, File, ...
		// apply: if the CheckApply happened in "apply" mode
		// eventful: did the CheckApply generate an event
		// errorful: did the CheckApply generate an error
		[]string{"kind", "apply", "eventful", "errorful"},
	)
	prometheus.MustRegister(obj.checkApplyTotal)

	obj.processStartTimeSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mgmt_process_start_time_seconds",
			Help: "Start time of the process since unix epoch in seconds.",
		},
	)
	prometheus.MustRegister(obj.processStartTimeSeconds)
	// directly set the processStartTimeSeconds
	obj.processStartTimeSeconds.SetToCurrentTime()

	return nil
}

// Start runs a http server in a go routine, that responds to /metrics
// as prometheus would expect.
func (obj *Prometheus) Start() error {
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(obj.Listen, nil)
	return nil
}

// Stop the http server.
func (obj *Prometheus) Stop() error {
	// TODO: There is no way in go < 1.8 to stop a http server.
	// https://stackoverflow.com/questions/39320025/go-how-to-stop-http-listenandserve/41433555#41433555
	return nil
}

// UpdateCheckApplyTotal refreshes the failing gauge by parsing the internal
// state map.
func (obj *Prometheus) UpdateCheckApplyTotal(kind string, apply, eventful, errorful bool) error {
	labels := prometheus.Labels{"kind": kind, "apply": strconv.FormatBool(apply), "eventful": strconv.FormatBool(eventful), "errorful": strconv.FormatBool(errorful)}
	metric := obj.checkApplyTotal.With(labels)
	metric.Inc()
	return nil
}

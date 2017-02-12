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
	"log"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// DefaultPrometheusListen is registered in
// https://github.com/prometheus/prometheus/wiki/Default-port-allocations
const DefaultPrometheusListen = "127.0.0.1:9233"

type ResState int

const (
	ResStateOK	ResState = iota
	ResStateSoftFail
	ResStateHardFail
)

// Prometheus is the struct that contains information about the
// prometheus instance. Run Init() on it.
type Prometheus struct {
	Listen string // the listen specification for the net/http server

	managedResources *prometheus.GaugeVec
	checkApplyCounter prometheus.Collector
	failedResourcesTotal prometheus.Collector
	failedResources prometheus.Collector

	resourcesState map[string]ResState
}

// Init some parameters - currently the Listen address.
func (obj *Prometheus) Init() error {
	if len(obj.Listen) == 0 {
		obj.Listen = DefaultPrometheusListen
	}

	obj.resourcesState = make(map[string]ResState)

	obj.managedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_resources",
			Help: "Number of managed resources.",
		},
		[]string{"type"}, // File, Svc, ...
	)
	log.Printf("xxxc %v", obj.managedResources)
	prometheus.MustRegister(obj.managedResources)

	obj.checkApplyCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_checkapply_total",
			Help: "Number of CheckApply that have run.",
		},
		[]string{"type"}, // File, Svc, ...
	)
	prometheus.MustRegister(obj.checkApplyCounter)

	obj.failedResourcesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_failures_total",
			Help: "Total of failed resources.",
		},
		[]string{"type", "kind"}, // File, Svc, ... |  soft, hard
	)
	prometheus.MustRegister(obj.failedResourcesTotal)

	obj.failedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_failures",
			Help: "Number of failing resources.",
		},
		[]string{"type", "kind"}, // File, Svc, ... | soft, hard
	)
	prometheus.MustRegister(obj.failedResources)

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

func (obj *Prometheus) AddManagedResource(resUuid string, rtype string) error {
	obj.managedResources.With(prometheus.Labels{"type": rtype}).Inc()
	obj.UpdateState(resUuid, ResStateOK)
	return nil
}

func (obj *Prometheus) RemoveManagedResource(resUuid string, rtype string) error {
	obj.managedResources.With(prometheus.Labels{"type": rtype}).Dec()
	if err := obj.deleteState(resUuid); err != nil {
	
	}
	return nil
}

func (obj *Prometheus) deleteState(resUuid string) error {
	delete obj.resourcesState[resUuid]
	return nil
}

func (obj *Prometheus) UpdateState(resUuid string, newState string) error {
	obj.resourcesState[resUuid] = newState
	return nil
}


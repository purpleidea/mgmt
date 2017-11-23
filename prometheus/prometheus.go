// Mgmt
// Copyright (C) 2013-2017+ James Shubin and the project contributors
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

// Package prometheus provides functions that are useful to control and manage
// the built-in prometheus instance.
package prometheus

import (
	"errors"
	"net/http"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	errwrap "github.com/pkg/errors"
)

// DefaultPrometheusListen is registered in
// https://github.com/prometheus/prometheus/wiki/Default-port-allocations
const DefaultPrometheusListen = "127.0.0.1:9233"

// ResState represents the status of a resource.
type ResState int

const (
	// ResStateOK represents a working resource
	ResStateOK ResState = iota
	// ResStateSoftFail represents a resource in soft fail (will be retried)
	ResStateSoftFail
	// ResStateHardFail represents a resource in hard fail (will NOT be retried)
	ResStateHardFail
)

// Prometheus is the struct that contains information about the
// prometheus instance. Run Init() on it.
type Prometheus struct {
	Listen string // the listen specification for the net/http server

	checkApplyTotal        *prometheus.CounterVec // total of CheckApplies that have been triggered
	pgraphStartTimeSeconds prometheus.Gauge       // process start time in seconds since unix epoch
	managedResources       *prometheus.GaugeVec   // Resources we manage now
	failedResourcesTotal   *prometheus.CounterVec // Total of failures since mgmt has started
	failedResources        *prometheus.GaugeVec   // Number of current resources

	resourcesState map[string]resStateWithKind // Maps the resources with their current kind/state
	mutex          *sync.Mutex                 // Mutex used to update resourcesState
}

// resStateWithKind is used to count the failures by kind
type resStateWithKind struct {
	state ResState
	kind  string
}

// Init some parameters - currently the Listen address.
func (obj *Prometheus) Init() error {
	if len(obj.Listen) == 0 {
		obj.Listen = DefaultPrometheusListen
	}

	obj.mutex = &sync.Mutex{}
	obj.resourcesState = make(map[string]resStateWithKind)

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

	obj.pgraphStartTimeSeconds = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "mgmt_graph_start_time_seconds",
			Help: "Start time of the current graph since unix epoch in seconds.",
		},
	)
	prometheus.MustRegister(obj.pgraphStartTimeSeconds)

	obj.managedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_resources",
			Help: "Number of managed resources.",
		},
		// kind: resource type: Svc, File, ...
		[]string{"kind"},
	)
	prometheus.MustRegister(obj.managedResources)

	obj.failedResourcesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mgmt_failures_total",
			Help: "Total of failed resources.",
		},
		// kind: resource type: Svc, File, ...
		// failure: soft or hard
		[]string{"kind", "failure"},
	)
	prometheus.MustRegister(obj.failedResourcesTotal)

	obj.failedResources = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mgmt_failures",
			Help: "Number of failing resources.",
		},
		// kind: resource type: Svc, File, ...
		// failure: soft or hard
		[]string{"kind", "failure"},
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

// InitKindMetrics initialized prometheus counters. For each kind of
// resource checkApply counters are initialized with all the possible value.
func (obj *Prometheus) InitKindMetrics(kinds []string) error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	bools := []bool{true, false}
	for _, kind := range kinds {
		for _, apply := range bools {
			for _, eventful := range bools {
				for _, errorful := range bools {
					labels := prometheus.Labels{
						"kind":     kind,
						"apply":    strconv.FormatBool(apply),
						"eventful": strconv.FormatBool(eventful),
						"errorful": strconv.FormatBool(errorful),
					}
					obj.checkApplyTotal.With(labels)
				}
			}
		}

		obj.managedResources.With(prometheus.Labels{"kind": kind})

		failures := []string{"soft", "hard"}
		for _, f := range failures {
			failLabels := prometheus.Labels{"kind": kind, "failure": f}
			obj.failedResourcesTotal.With(failLabels)
			obj.failedResources.With(failLabels)
		}
	}
	return nil
}

// UpdateCheckApplyTotal refreshes the failing gauge by parsing the internal
// state map.
func (obj *Prometheus) UpdateCheckApplyTotal(kind string, apply, eventful, errorful bool) error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	labels := prometheus.Labels{"kind": kind, "apply": strconv.FormatBool(apply), "eventful": strconv.FormatBool(eventful), "errorful": strconv.FormatBool(errorful)}
	metric := obj.checkApplyTotal.With(labels)
	metric.Inc()
	return nil
}

// UpdatePgraphStartTime updates the mgmt_graph_start_time_seconds metric
// to the current timestamp.
func (obj *Prometheus) UpdatePgraphStartTime() error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	obj.pgraphStartTimeSeconds.SetToCurrentTime()
	return nil
}

// AddManagedResource increments the Managed Resource counter and updates the resource status.
func (obj *Prometheus) AddManagedResource(resUUID string, rtype string) error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	obj.managedResources.With(prometheus.Labels{"kind": rtype}).Inc()
	if err := obj.UpdateState(resUUID, rtype, ResStateOK); err != nil {
		return errwrap.Wrapf(err, "can't update the resource status in the map")
	}
	return nil
}

// RemoveManagedResource decrements the Managed Resource counter and updates the resource status.
func (obj *Prometheus) RemoveManagedResource(resUUID string, rtype string) error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	obj.managedResources.With(prometheus.Labels{"kind": rtype}).Dec()
	if err := obj.deleteState(resUUID); err != nil {
		return errwrap.Wrapf(err, "can't remove the resource status from the map")
	}
	return nil
}

// deleteState removes the resources for the state map and re-populates the failing gauge.
func (obj *Prometheus) deleteState(resUUID string) error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	obj.mutex.Lock()
	delete(obj.resourcesState, resUUID)
	obj.mutex.Unlock()
	if err := obj.updateFailingGauge(); err != nil {
		return errwrap.Wrapf(err, "can't update the failing gauge")
	}
	return nil
}

// UpdateState updates the state of the resources in our internal state map
// then triggers a refresh of the failing gauge.
func (obj *Prometheus) UpdateState(resUUID string, rtype string, newState ResState) error {
	defer obj.updateFailingGauge()
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	obj.mutex.Lock()
	obj.resourcesState[resUUID] = resStateWithKind{state: newState, kind: rtype}
	obj.mutex.Unlock()
	if newState != ResStateOK {
		var strState string
		if newState == ResStateSoftFail {
			strState = "soft"
		} else if newState == ResStateHardFail {
			strState = "hard"
		} else {
			return errors.New("state should be soft or hard failure")
		}
		obj.failedResourcesTotal.With(prometheus.Labels{"kind": rtype, "failure": strState}).Inc()
	}
	return nil
}

// updateFailingGauge refreshes the failing gauge by parsking the internal
// state map.
func (obj *Prometheus) updateFailingGauge() error {
	if obj == nil {
		return nil // happens when mgmt is launched without --prometheus
	}
	var softFails, hardFails map[string]float64
	softFails = make(map[string]float64)
	hardFails = make(map[string]float64)
	for _, v := range obj.resourcesState {
		if v.state == ResStateSoftFail {
			softFails[v.kind]++
		} else if v.state == ResStateHardFail {
			hardFails[v.kind]++
		}
	}
	// TODO: we might want to Zero the metrics we are not using
	// because in prometheus design the metrics keep living for some time
	// even after they are removed.
	obj.failedResources.Reset()
	for k, v := range softFails {
		obj.failedResources.With(prometheus.Labels{"kind": k, "failure": "soft"}).Set(v)
	}
	for k, v := range hardFails {
		obj.failedResources.With(prometheus.Labels{"kind": k, "failure": "hard"}).Set(v)
	}
	return nil
}
